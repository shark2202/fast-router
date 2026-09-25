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
# Requires: go, zig, curl (for llama.cpp nightly download)
set -e

VERSION="${VERSION:-0.1.0}"
OUTDIR="${OUTDIR:-dist}"
LLAMA_VER="${LLAMA_VER:-b11175}"

mkdir -p "$OUTDIR"

# Platforms to build (Go binary + zig wrapper)
PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
  "windows/arm64"
)

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
  case "$os" in
    darwin) echo "${arch}-macos" ;;
    linux)  echo "${arch}-linux-gnu" ;;
    windows) echo "${arch}-windows-msvc" ;;
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

echo "=== building fast-router ${VERSION} ==="

for plat in "${PLATFORMS[@]}"; do
  os="${plat%/*}"
  arch="${plat#*/}"
  ext=""
  [ "$os" = "windows" ] && ext=".exe"
  pkgname="fast-router-${VERSION}-${os}-${arch}"
  workdir="${OUTDIR}/${pkgname}"
  mkdir -p "$workdir/lib"

  echo "--- ${plat} ---"

  # 1. Go binary (cross-compile, no cgo)
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$workdir/fast-router${ext}" ./cmd/fast-router

  # 2. zig wrapper (cross-compile if zig available, else skip with warning)
  ext_lib=$(lib_ext "$plat")
  if command -v zig >/dev/null 2>&1; then
    target=$(zig_target "$plat")
    zig build-lib zig/frwrapper.zig -dynamic \
      -Izig/include \
      -target "$target" \
      -O ReleaseFast 2>/dev/null \
      -o "$workdir/lib/libfrwrapper.${ext_lib}" \
      || echo "  (zig cross-compile failed for $plat, user must build from source)"
  fi

  # 3. llama.cpp prebuilt (download + extract libs)
  pkg=$(llama_pkg "$plat")
  llama_url="https://github.com/ggml-org/llama.cpp/releases/download/${LLAMA_VER}/${pkg}"
  echo "  downloading ${pkg}..."
  if curl -sL -o "/tmp/${pkg}" "$llama_url" 2>/dev/null; then
    case "$pkg" in
      *.tar.gz)
        tar -xzf "/tmp/${pkg}" -C "/tmp/llama-extract-${plat}" 2>/dev/null || \
          mkdir -p "/tmp/llama-extract-${plat}" && tar -xzf "/tmp/${pkg}" -C "/tmp/llama-extract-${plat}"
        cp "/tmp/llama-extract-${plat}"/*/libllama.* "$workdir/lib/" 2>/dev/null || true
        cp "/tmp/llama-extract-${plat}"/*/libggml*.* "$workdir/lib/" 2>/dev/null || true
        ;;
      *.zip)
        mkdir -p "/tmp/llama-extract-${plat}"
        unzip -oq "/tmp/${pkg}" -d "/tmp/llama-extract-${plat}" 2>/dev/null || true
        cp "/tmp/llama-extract-${plat}"/*/libllama.* "$workdir/lib/" 2>/dev/null || true
        cp "/tmp/llama-extract-${plat}"/*/libggml*.* "$workdir/lib/" 2>/dev/null || true
        ;;
    esac
    rm -rf "/tmp/${pkg}" "/tmp/llama-extract-${plat}"
  else
    echo "  (download failed, user must download llama.cpp nightly separately)"
  fi

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

  # 5. zip
  (cd "$OUTDIR" && zip -qr "${pkgname}.zip" "${pkgname}/")
  rm -rf "$workdir"
  echo "  → ${OUTDIR}/${pkgname}.zip"
done

echo ""
echo "=== done. packages in ${OUTDIR}/ ==="
ls -lh ${OUTDIR}/*.zip 2>/dev/null
