#!/bin/bash
# scripts/pack.sh — build cross-platform fast-router distribution zips.
#
# Produces fast-router-{platform}-{arch}.zip containing:
#   fast-router          (Go binary, cross-compiled)
#   lib/libfrwrapper.*   (zig wrapper, cross-compiled)
#   lib/libllama.*       (llama.cpp prebuilt, from nightly release)
#   lib/libggml*.*       (llama.cpp deps)
#   fast-router.example.json
#   README.txt
#
# Usage: ./scripts/pack.sh
# Requires: go, zig 0.14.x, tar/unzip, zip, objdump (Windows targets)
set -e

VERSION="${VERSION:-0.1.0}"
OUTDIR="${OUTDIR:-dist}"
LLAMA_VER="${LLAMA_VER:-b11175}"
OFFLINE="${OFFLINE:-0}"
LLAMA_ARCHIVE_DIR="${LLAMA_ARCHIVE_DIR:-.cache/llama/${LLAMA_VER}}"
ZIG_BIN="${ZIG_BIN:-zig}"
OBJDUMP_BIN="${OBJDUMP_BIN:-objdump}"
TMP_ROOT="${TMP_ROOT:-${TMPDIR:-/tmp}/fast-router-pack-$$}"
CURRENT_WORKDIR=""

mkdir -p "$OUTDIR"
mkdir -p "$TMP_ROOT"
cleanup() {
  [ -z "$CURRENT_WORKDIR" ] || rm -rf "$CURRENT_WORKDIR"
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

case "$OFFLINE" in
  0|1) ;;
  *) echo "OFFLINE must be 0 or 1" >&2; exit 1 ;;
esac

for required_cmd in go tar unzip zip; do
  command -v "$required_cmd" >/dev/null 2>&1 || {
    echo "missing required command: $required_cmd" >&2
    exit 1
  }
done
if [ "$OFFLINE" = "0" ]; then
  command -v curl >/dev/null 2>&1 || {
    echo "missing required command: curl (or set OFFLINE=1)" >&2
    exit 1
  }
fi
command -v "$ZIG_BIN" >/dev/null 2>&1 || {
  echo "missing Zig compiler: $ZIG_BIN" >&2
  exit 1
}
zig_version="$("$ZIG_BIN" version)"
case "$zig_version" in
  0.14.*) ;;
  *) echo "unsupported Zig version: $zig_version (expected 0.14.x)" >&2; exit 1 ;;
esac

# Platforms to build (Go binary + zig wrapper)
if [ -n "${PLATFORM_LIST:-}" ]; then
  read -r -a PLATFORMS <<< "$PLATFORM_LIST"
else
  PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
  )
fi

# llama.cpp nightly release package suffix per platform
llama_pkg() {
  local plat="$1"
  local os="${plat%/*}"
  local arch="${plat#*/}"
  case "$os/$arch" in
    darwin/amd64) echo "llama-${LLAMA_VER}-bin-macos-x64.tar.gz" ;;
    darwin/arm64) echo "llama-${LLAMA_VER}-bin-macos-arm64.tar.gz" ;;
    linux/amd64)  echo "llama-${LLAMA_VER}-bin-ubuntu-x64.tar.gz" ;;
    linux/arm64)  echo "llama-${LLAMA_VER}-bin-ubuntu-arm64.tar.gz" ;;
    windows/amd64) echo "llama-${LLAMA_VER}-bin-win-cpu-x64.zip" ;;
    windows/arm64) echo "llama-${LLAMA_VER}-bin-win-cpu-arm64.zip" ;;
  esac
}

# zig target triple for cross-compile
zig_target() {
  local plat="$1"
  local os="${plat%/*}"
  local arch="${plat#*/}"
  local zig_arch
  case "$arch" in
    amd64) zig_arch="x86_64" ;;
    arm64) zig_arch="aarch64" ;;
    *) echo "unsupported architecture: $arch" >&2; return 1 ;;
  esac
  case "$os" in
    darwin) echo "${zig_arch}-macos" ;;
    linux)  echo "${zig_arch}-linux-gnu" ;;
    windows) echo "${zig_arch}-windows-gnu" ;;
    *) echo "unsupported OS: $os" >&2; return 1 ;;
  esac
}

# lib extension per platform
lib_ext() {
  case "$1" in
    darwin/*) echo "dylib" ;;
    linux/*)  echo "so" ;;
    windows/*) echo "dll" ;;
  esac
}

ggml_cpu_lib() {
  case "$1" in
    darwin/*) echo "ggml-cpu" ;;
    linux/amd64|windows/amd64) echo "ggml-cpu-x64" ;;
    linux/arm64) echo "ggml-cpu-armv8.0_1" ;;
    windows/arm64) echo "ggml-cpu" ;;
    *) echo "unsupported platform: $1" >&2; return 1 ;;
  esac
}

sysroot_for() {
  local plat="$1"
  local os="${plat%/*}"
  local arch="${plat#*/}"
  local var="ZIG_SYSROOT_$(printf '%s' "$os" | tr '[:lower:]' '[:upper:]')_$(printf '%s' "$arch" | tr '[:lower:]' '[:upper:]')"
  printf '%s\n' "${!var:-}"
}

windows_import_machine() {
  case "$1" in
    windows/amd64) echo "i386:x86-64" ;;
    windows/arm64) echo "arm64" ;;
    *) echo "unsupported Windows target: $1" >&2; return 1 ;;
  esac
}

generate_windows_import_lib() {
  local plat="$1"
  local dll="$2"
  local output_dir="$3"
  local dll_name
  local stem
  local def_file
  local import_lib
  local machine
  dll_name="$(basename "$dll")"
  stem="${dll_name%.dll}"
  def_file="$TMP_ROOT/${stem}.def"
  import_lib="$output_dir/${stem}.lib"
  machine=$(windows_import_machine "$plat")

  {
    printf 'LIBRARY %s\n' "$dll_name"
    printf 'EXPORTS\n'
    "$OBJDUMP_BIN" -p "$dll" | awk '
      /^Export Table:/ { in_exports = 1; next }
      in_exports && /^The Import Tables:/ { exit }
      in_exports && $1 ~ /^[0-9]+$/ && $2 ~ /^0x/ && $3 != "" { print $3 }
    '
  } > "$def_file"
  grep -q '^EXPORTS$' "$def_file"
  tail -n +3 "$def_file" | grep -q '[^[:space:]]'
  "$ZIG_BIN" dlltool -D "$dll_name" -d "$def_file" -l "$import_lib" -m "$machine"
  test -s "$import_lib"
}

download_llama() {
  local plat="$1"
  local pkg="$2"
  local archive="$TMP_ROOT/${pkg}"
  local extract_dir="$TMP_ROOT/extract-${plat//\//-}"
  local url="https://github.com/ggml-org/llama.cpp/releases/download/${LLAMA_VER}/${pkg}"

  rm -rf "$extract_dir"
  mkdir -p "$extract_dir"
  if [ "$OFFLINE" = "1" ]; then
    local local_archive="$LLAMA_ARCHIVE_DIR/$pkg"
    if [ ! -f "$local_archive" ]; then
      echo "  missing local llama archive: $local_archive" >&2
      return 1
    fi
    echo "  using local ${local_archive}..." >&2
    cp "$local_archive" "$archive"
  else
    echo "  downloading ${pkg}..." >&2
    curl --fail --location --retry 3 --silent --show-error \
      -o "$archive" "$url"
  fi

  case "$pkg" in
    *.tar.gz) tar -xzf "$archive" -C "$extract_dir" ;;
    *.zip) unzip -q "$archive" -d "$extract_dir" ;;
    *) echo "unsupported llama archive: $pkg" >&2; return 1 ;;
  esac

  printf '%s\n' "$extract_dir"
}

find_llama_dir() {
  local extract_dir="$1"
  local llama_lib
  llama_lib="$(
    find "$extract_dir" \( -type f -o -type l \) \
      \( -name 'libllama.*' -o -name 'libllama.*.*' -o -name 'llama.dll' -o -name 'llama.lib' \) \
      -print -quit
  )"
  if [ -z "$llama_lib" ]; then
    echo "  missing target llama library under $extract_dir" >&2
    return 1
  fi
  dirname "$llama_lib"
}

copy_runtime_libs() {
  local extract_dir="$1"
  local destination="$2"
  local found=0
  while IFS= read -r lib; do
    [ -z "$lib" ] && continue
    cp -a "$lib" "$destination/"
    found=1
  done < <(
    find "$extract_dir" \( -type f -o -type l \) \
      \( -name 'libllama*.dylib' -o -name 'libllama*.so*' -o \
         -name 'libggml*.dylib' -o -name 'libggml*.so*' -o \
         -name 'libllama*.dll' -o -name 'libggml*.dll' -o \
         -name 'llama*.dll' -o -name 'ggml*.dll' \) \
      -print
  )
  if [ "$found" -ne 1 ]; then
    echo "  no llama.cpp runtime libraries found under $extract_dir" >&2
    return 1
  fi
}

echo "=== building fast-router ${VERSION} ==="

for plat in "${PLATFORMS[@]}"; do
  os="${plat%/*}"
  arch="${plat#*/}"
  ext=""
  [ "$os" = "windows" ] && ext=".exe"
  pkgname="fast-router-${VERSION}-${os}-${arch}"
  workdir="${OUTDIR}/${pkgname}"
  CURRENT_WORKDIR="$workdir"
  mkdir -p "$workdir/lib"

  echo "--- ${plat} ---"

  # 1. Go binary (cross-compile, no cgo)
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$workdir/fast-router${ext}" ./cmd/fast-router

  target=$(zig_target "$plat")
  cpu_lib=$(ggml_cpu_lib "$plat")
  sysroot=$(sysroot_for "$plat")

  # 2. Download target-specific llama.cpp runtime/link libraries.
  pkg=$(llama_pkg "$plat")
  extract_dir=$(download_llama "$plat" "$pkg")
  llama_dir=$(find_llama_dir "$extract_dir")
  copy_runtime_libs "$extract_dir" "$workdir/lib"
  if [ "$os" = "windows" ]; then
    command -v "$OBJDUMP_BIN" >/dev/null 2>&1 || {
      echo "missing PE export tool: $OBJDUMP_BIN" >&2
      exit 1
    }
    for dll_name in llama.dll ggml-base.dll "${cpu_lib}.dll"; do
      dll_path=$(find "$extract_dir" -type f -name "$dll_name" -print -quit)
      if [ -z "$dll_path" ]; then
        echo "  missing Windows DLL: $dll_name" >&2
        exit 1
      fi
      generate_windows_import_lib "$plat" "$dll_path" "$llama_dir"
      cp -a "$llama_dir/$(basename "${dll_name%.dll}").lib" "$workdir/lib/"
    done
  fi

  # 3. Build the Zig wrapper against the target-specific llama.cpp libraries.
  ext_lib=$(lib_ext "$plat")
  zig_prefix="$TMP_ROOT/zig-${plat//\//-}"
  zig_args=(
    build
    "-Dtarget=$target"
    "-Dllama_dir=$llama_dir"
    "-Dggml_cpu_lib=$cpu_lib"
    -Doptimize=ReleaseFast
    --prefix "$zig_prefix"
  )
  if [ -n "$sysroot" ]; then
    zig_args+=("-Dsysroot=$sysroot")
  fi
  (
    cd zig
    "$ZIG_BIN" "${zig_args[@]}"
  )
  wrapper_root="$zig_prefix/lib"
  [ "$os" = "windows" ] && wrapper_root="$zig_prefix/bin"
  wrapper=$(find "$wrapper_root" \
    \( -name "libfrwrapper.${ext_lib}" -o -name "frwrapper.${ext_lib}" \) \
    -print -quit)
  if [ -z "$wrapper" ]; then
    echo "  Zig build completed but libfrwrapper.${ext_lib} was not produced" >&2
    exit 1
  fi
  cp -a "$wrapper" "$workdir/lib/libfrwrapper.${ext_lib}"

  # 4. config template + README
  cp fast-router.example.json "$workdir/"
  cat > "$workdir/README.txt" <<EOF
fast-router ${VERSION} (${os}/${arch})

Quick start:
  1. Set your API keys: edit fast-router.json (or use the admin UI)
  2. Set DYLD_LIBRARY_PATH (macOS) / LD_LIBRARY_PATH (linux) / PATH (windows) to ./lib
  3. Run: ./fast-router --config fast-router.json
  4. Open http://localhost:8080/admin to configure upstreams + model
  5. Point your client (codex/claude code/pi-agent) base_url to http://localhost:8080

For Jev smart routing: download a GGUF model (e.g. Qwen2.5-1.5B-Instruct-Q4_K_M.gguf
from modelscope) and set model.path in the config.
EOF

  # 5. Validate required files before creating the ZIP.
  test -s "$workdir/fast-router${ext}"
  test -s "$workdir/lib/libfrwrapper.${ext_lib}"
  find "$workdir/lib" \( -type f -o -type l \) -print -quit | grep -q .

  # 6. zip
  (cd "$OUTDIR" && zip -qr "${pkgname}.zip" "${pkgname}/")
  rm -rf "$workdir"
  CURRENT_WORKDIR=""
  echo "  → ${OUTDIR}/${pkgname}.zip"
done

echo ""
echo "=== done. packages in ${OUTDIR}/ ==="
ls -lh ${OUTDIR}/*.zip 2>/dev/null
