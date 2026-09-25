# fast-router 构建·打包·分发 SOP

> **版本**: 0.1.0 · **更新**: 2026-09-25
>
> 本文档是 fast-router 从源码到用户手中的完整标准作业流程（SOP）。
> 分三个阶段：**构建**（Go binary + zig wrapper）→ **打包**（zip + llama.cpp 预编译库 + config 模板）→ **分发**（用户下载→解压→首启→admin UI 配置）。

---

## 目录

1. [前置依赖](#1-前置依赖)
2. [构建阶段](#2-构建阶段)
3. [打包阶段](#3-打包阶段)
4. [分发阶段（用户侧）](#4-分发阶段用户侧)
5. [验证检查清单](#5-验证检查清单)
6. [故障排查](#6-故障排查)

---

## 1. 前置依赖

### 构建机需要

| 工具 | 版本 | 用途 | 验证 |
|------|------|------|------|
| **Go** | ≥ 1.25 | 编译 fast-router binary（CGO_ENABLED=0） | `go version` |
| **Zig** | ≥ 0.14 | 编译 libfrwrapper（@cImport llama.h） | `zig version` |
| **curl** | 任意 | 下载 llama.cpp nightly 预编译包 | `curl --version` |
| **zip** | 任意 | 打包分发 zip | `zip --version` |

### 不需要的

- ❌ C 编译器（不用 cgo）
- ❌ Python（构建时不需要；用户首启下模型时需要 modelscope SDK）
- ❌ llama.cpp 源码（用预编译 nightly binary）
- ❌ 模型权重（用户首启时按需下载）

### 关键设计决策

- **CGO_ENABLED=0**：Go binary 不用 cgo，可从单一源码交叉编译到 6 平台
- **purego**（Unix）/ **syscall.NewLazyDLL**（Windows）：dlopen libfrwrapper 不用 cgo
- **zig wrapper**：@cImport llama.h，struct 由 zig 编译器处理（消解 ABI 风险）
- **llama.cpp nightly**：预编译共享库（libllama + libggml），不从源码编译
- **模型不在 zip 里**：用户首启通过 admin UI 下载（modelscope SDK）

---

## 2. 构建阶段

### 2.1 Go binary（交叉编译，6 平台）

```bash
# 单一源码，交叉编译到所有目标平台
for plat in "darwin/amd64" "darwin/arm64" "linux/amd64" "linux/arm64" "windows/amd64" "windows/arm64"; do
  os=${plat%/*}; arch=${plat#*/}; ext=""; [ "$os" = "windows" ] && ext=".exe"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -o "fast-router-${os}-${arch}${ext}" ./cmd/fast-router
done
```

- 不用 cgo（`CGO_ENABLED=0`）
- 单二进制 8-9MB
- Unix 用 purego（dlopen），Windows 用 syscall.NewLazyDLL（LoadDLL）
- 6 平台从单一源码交叉编译

### 2.2 Zig wrapper（libfrwrapper）

```bash
# 本机构建（macOS x64 示例）
cd zig
zig build-lib frwrapper.zig -dynamic \
  -Iinclude \
  -L/path/to/llama-bins/llama-b11175 \
  -lllama -lggml-base -lggml-cpu \
  -O ReleaseFast
```

| 参数 | 说明 |
|------|------|
| `-dynamic` | 输出共享库（.dylib/.so/.dll） |
| `-Iinclude` | 头文件目录（llama.h + ggml*.h，vendored） |
| `-L<path>` | llama.cpp 预编译库目录 |
| `-lllama` | 链接 libllama（注意：两个 l） |
| `-lggml-base -lggml-cpu` | 链接 ggml 依赖 |
| `-O ReleaseFast` | 优化 |

**交叉编译**（zig 的强项）：
```bash
zig build-lib frwrapper.zig -dynamic -Iinclude \
  -target aarch64-linux-gnu \
  -L/path/to/linux-libs \
  -lllama -lggml-base -lggml-cpu -O ReleaseFast
```

Zig target 映射：

| 目标平台 | zig target |
|----------|------------|
| darwin/amd64 | `x86_64-macos` |
| darwin/arm64 | `aarch64-macos` |
| linux/amd64 | `x86_64-linux-gnu` |
| linux/arm64 | `aarch64-linux-gnu` |
| windows/amd64 | `x86_64-windows-msvc` |
| windows/arm64 | `aarch64-windows-msvc` |

### 2.3 llama.cpp 预编译库（不用编译）

从 nightly release 下载，每平台一个包：

```bash
LLAMA_VER=b11175

# 平台 → 包名映射
darwin/amd64 → llama-${LLAMA_VER}-bin-macos-x64.tar.gz
darwin/arm64 → llama-${LLAMA_VER}-bin-macos-arm64.tar.gz
linux/amd64  → llama-${LLAMA_VER}-bin-ubuntu-x64.tar.gz
linux/arm64  → llama-${LLAMA_VER}-bin-ubuntu-arm64.tar.gz
windows/amd64 → llama-${LLAMA_VER}-bin-win-cpu-x64.zip
windows/arm64 → llama-${LLAMA_VER}-bin-win-cpu-arm64.zip

# 下载 + 解压
curl -sL -o pkg.tar.gz "https://github.com/ggml-org/llama.cpp/releases/download/${LLAMA_VER}/${pkg}"
tar -xzf pkg.tar.gz    # 或 unzip
# 包内含：libllama.{so,dylib,dll} + libggml*.{so,dylib,dll} + 可执行
```

**验证包内含共享库**：
```bash
tar -tzf pkg.tar.gz | grep libllama
# 应看到 libllama.dylib/.so + libggml*.dylib/.so
```

### 2.4 测试构建

```bash
# Go 单元测试（30 测试）
go test ./router/... ./router/schema/...
# 应输出：ok  fast-router/router  + ok  fast-router/router/schema

# Jev benchmark（需要 GGUF 模型 + libfrwrapper + libllama）
DYLD_LIBRARY_PATH=/path/to/llama-libs \
  go run ./cmd/jevbench /path/to/model.gguf
```

---

## 3. 打包阶段

### 3.1 自动打包脚本

```bash
./scripts/pack.sh
```

脚本自动完成（6 平台）：
1. Go 交叉编译 binary
2. Zig 交叉编译 libfrwrapper
3. 下载 llama.cpp nightly 预编译包 + 解压 libllama/libggml
4. 组装 zip（binary + lib/ + config 模板 + README）
5. 输出到 `dist/`

### 3.2 手动打包（单平台示例：darwin/amd64）

```bash
PKG=fast-router-0.1.0-darwin-amd64
mkdir -p $PKG/lib

# 1. Go binary
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $PKG/fast-router ./cmd/fast-router

# 2. Zig wrapper
cd zig && zig build-lib frwrapper.zig -dynamic -Iinclude \
  -L/path/to/llama-bins -lllama -lggml-base -lggml-cpu -O ReleaseFast \
  -o ../$PKG/lib/libfrwrapper.dylib && cd ..

# 3. llama.cpp 共享库
cp /path/to/llama-bins/libllama.dylib $PKG/lib/
cp /path/to/llama-bins/libggml*.dylib $PKG/lib/

# 4. config 模板（不含 API key）
cp fast-router.example.json $PKG/

# 5. README
cat > $PKG/README.txt <<'EOF'
fast-router 0.1.0 (darwin/amd64)

Quick start:
  1. Set API keys: edit fast-router.json or use http://localhost:8080/admin
  2. Set DYLD_LIBRARY_PATH=./lib (macOS) / LD_LIBRARY_PATH=./lib (Linux) / PATH.../lib (Windows)
  3. Run: ./fast-router --config fast-router.json
  4. Open http://localhost:8080/admin → configure upstreams + download model
  5. Point client base_url to http://localhost:8080
EOF

# 6. zip
zip -r $PKG.zip $PKG/
```

### 3.3 zip 内容

```
fast-router-{ver}-{os}-{arch}.zip
├── fast-router              ← Go binary (8-9MB, CGO_ENABLED=0)
├── lib/
│   ├── libfrwrapper.{dylib,so,dll}  ← zig wrapper (~1MB)
│   ├── libllama.{dylib,so,dll}      ← llama.cpp 推理引擎
│   └── libggml*.{dylib,so,dll}      ← llama.cpp 依赖
├── fast-router.example.json  ← config 模板（无 API key）
└── README.txt
```

**总大小**: ~10-15MB（Go binary + zig wrapper + llama.cpp libs）
**不含**: 模型权重（用户首启下载，0.5B ~0.5GB / 8B ~5GB）

### 3.4 平台特定注意事项

| 平台 | 库路径环境变量 | 备注 |
|------|----------------|------|
| macOS | `DYLD_LIBRARY_PATH=./lib` | SIP 限制：不能在 /usr/lib 放自定义库 |
| Linux | `LD_LIBRARY_PATH=./lib` | 标准 |
| Windows | 把 `lib/` 加入 PATH 或放 exe 同目录 | Windows dll 不需环境变量（同目录自动加载） |

---

## 4. 分发阶段（用户侧）

### 4.1 用户操作流程

```
1. 下载 zip → 解压
2. 设置库路径:
   macOS:  export DYLD_LIBRARY_PATH=./lib
   Linux:  export LD_LIBRARY_PATH=./lib
   Windows: （lib/ 在 exe 同目录，无需设置）
3. 启动:
   ./fast-router --config fast-router.json
   （首次启动自动生成默认 config 模板）
4. 打开浏览器: http://localhost:8080/admin
5. 配置:
   a. Upstreams: 填 API key（OpenAI/Anthropic/DeepSeek/自定义）
   b. Registry: 配置 model_id（匹配 upstream 认的模型名）+ capability
   c. Model: 选 GGUF 模型 → 点 download（modelscope SDK 下载）
6. 保存 → Jev scorer 就绪
7. 客户端配 base_url:
   codex:        OPENAI_BASE_URL=http://localhost:8080/v1
   claude code:  ANTHROPIC_BASE_URL=http://localhost:8080
   pi-agent:     配置指向 localhost:8080
```

### 4.2 两种运行模式

| 模式 | 条件 | 行为 |
|------|------|------|
| **hint-only** | config.model.path 为空 | 不加载 Jev scorer，按 model 名 strong hint 或 default upstream 转发（仍可用，不智能路由） |
| **Jev 智能路由** | config.model.path 指向 GGUF | C3 Jev 分类任务类型 → C4 挡死 → C5 选模 → 转发 |

用户可以先 hint-only 模式启动（配 upstream 即可用），后续通过 admin UI 下载模型启用 Jev 智能路由。

### 4.3 客户端配置

#### codex（OpenAI 格式）
```bash
# 设置环境变量指向 fast-router
export OPENAI_BASE_URL=http://localhost:8080/v1
export OPENAI_API_KEY=any  # fast-router 不验证客户端 key（upstream key 在 admin UI 配）
# codex 的 config.toml 里 model 改为任意名（如 "fast-router"）或 upstream/model_id
```

#### claude code（Anthropic 格式）
```bash
export ANTHROPIC_BASE_URL=http://localhost:8080
export ANTHROPIC_API_KEY=any
```

#### pi-agent
```bash
# pi-agent 配置 base_url 指向 localhost:8080
```

### 4.4 admin UI 功能

| 路径 | 功能 |
|------|------|
| `/admin` | HTML 配置页 |
| `/api/config` GET | 读取 config |
| `/api/config` POST | 更新 config（持久化 + 热重载） |
| `/api/registry` GET/POST | 模型 registry CRUD（热重载） |
| `/api/models` GET | 可选 GGUF 模型列表 |
| `/api/models/download` POST | 启动模型下载（modelscope） |
| `/api/models/download/status` GET | 下载进度 |
| `/api/upstream/test` POST | 测试 upstream 连通性 |

---

## 5. 验证检查清单

### 构建后

- [ ] `go test ./router/... ./router/schema/...` 全 PASS（30 测试）
- [ ] `CGO_ENABLED=0 go build ./cmd/fast-router` 成功
- [ ] 6 平台交叉编译成功（darwin/linux/windows × amd64/arm64）
- [ ] `zig build-lib` 生成 libfrwrapper.{dylib,so,dll}
- [ ] llama.cpp nightly 包含 libllama + libggml 共享库

### 打包后

- [ ] zip 解压后含 fast-router + lib/ + fast-router.example.json + README.txt
- [ ] zip 大小 ~10-15MB
- [ ] `./fast-router --config fast-router.example.json` 能启动
- [ ] `http://localhost:8080/admin` 能打开
- [ ] `/api/config` GET 返回 JSON
- [ ] `/api/models` GET 返回模型列表
- [ ] `DYLD_LIBRARY_PATH=./lib`（macOS）设好后能加载 Jev scorer

### 分发后（用户侧）

- [ ] 解压 → 启动 → admin UI 可访问
- [ ] 配 upstream API key → test 连通性 OK
- [ ] 下载 GGUF 模型 → config.model.path 自动设
- [ ] `curl http://localhost:8080/v1/chat/completions -d '{"model":"fast-router","messages":[...]}'` 返回 LLM 响应
- [ ] codex/claude code base_url 指向 localhost:8080 → agent 任务正常

---

## 6. 故障排查

### dlopen 失败（libfrwrapper 找不到）

```
backend: dlopen(zig/libfrwrapper.dylib): tried: ... (no such file)
```

**原因**: 库路径未设或 libfrwrapper 未构建。

**修复**:
```bash
# macOS
export DYLD_LIBRARY_PATH=/path/to/lib
# Linux
export LD_LIBRARY_PATH=/path/to/lib
# Windows: 把 lib/ 放在 fast-router.exe 同目录

# 如果 libfrwrapper 不存在，构建：
cd zig
zig build-lib frwrapper.zig -dynamic -Iinclude -L/path/to/llama-libs -lllama -lggml-base -lggml-cpu
```

### zig build-lib 链接错误

```
error: 'ggml.h' file not found
```

**原因**: @cImport 的头文件依赖链未满足。

**修复**: 确认 `zig/include/` 含所有头文件：
```bash
ls zig/include/  # 应有: llama.h ggml.h ggml-backend.h ggml-cpu.h ggml-alloc.h ggml-opt.h gguf.h
```

### llama.cpp 包下载失败

**修复**: 手动下载对应平台的 nightly 包：
```bash
# 查最新 nightly tag
curl -s "https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=3" | grep tag_name | head -3
# 下载（替换 b11175 为最新）
curl -L -o pkg.tar.gz "https://github.com/ggml-org/llama.cpp/releases/download/b11175/llama-b11175-bin-macos-x64.tar.gz"
```

### modelscope 模型下载慢

**原因**: modelscope.cn 网速波动（279kB/s - 5MB/s）。

**修复**:
- 耐心等（0.5B ~10min / 1.5B ~30min / 8B ~5h）
- 或手动下载：`pip install modelscope && python -c "from modelscope import snapshot_download; print(snapshot_download('Qwen/Qwen2.5-0.5B-Instruct-GGUF', allow_patterns=['*Q4_K_M*','*.json']))"`
- 把路径填入 config.model.path

### 4router.net "model not found"

**原因**: registry 的 model_id 不匹配 upstream 认的模型名。

**修复**: 在 admin UI → registry → 编辑 model_id 为 upstream 认的模型名（如 `gpt-6-luna`）→ save。

### SSE 转换问题

**症状**: Anthropic 客户端收到空响应或格式错误。

**检查**:
```bash
# 测试 SSE 转换
curl -sN http://localhost:8080/v1/messages \
  -H 'Content-Type: application/json' -H 'x-api-key: dummy' \
  -d '{"model":"...","messages":[{"role":"user","content":"hi"}],"max_tokens":10,"stream":true}'
# 应看到 event: message_start → content_block_delta → message_stop
```

### 404 page not found

**检查**:
- 确认 fast-router 在运行（`curl http://localhost:8080/api/config`）
- 确认端口没被旧进程占用（`pkill -f fast-router`）
- 确认 mux 路由（main.go: `/v1/` → gateway, `/api/` → admin, `/admin` → admin）

---

## 附录：文件结构

```
fast-router/
├── cmd/
│   ├── fast-router/main.go     ← 入口（config 加载 + gateway + admin 启动）
│   ├── jevbench/main.go        ← P1 benchmark（30 样本）
│   └── zigbench/main.go        ← 单样本调试
├── router/
│   ├── gateway.go              ← C1/C6 HTTP 双端点 + 路由链 + forward + schema 转换
│   ├── scorer.go               ← C3 Jev system-one scorer（Choice + softmax）
│   ├── zig_backend.go          ← C3 Backend（Unix: purego dlopen）
│   ├── zig_backend_windows.go  ← C3 Backend（Windows: syscall.NewLazyDLL）
│   ├── turn_detector.go        ← C2 任务轮判定
│   ├── matcher.go              ← C4 声明能力挡死
│   ├── selector.go             ← C5 实测+成本加权选模
│   ├── task_types.go           ← 10 种子任务类型 + capability
│   ├── registry.go             ← ModelEntry + SeedRegistry
│   ├── config.go               ← Config JSON 持久化
│   ├── admin.go                ← admin web UI（upstreams + registry + 模型下载）
│   ├── model_download.go       ← GGUF 模型选择 + 异步下载
│   ├── modelscope.go           ← modelscope SDK 下载
│   ├── errors.go               ← frScoreError 映射
│   └── schema/                 ← OpenAI↔Anthropic 转换
│       ├── schema.go           ← ConvertRequest/Response + SSEConverter
│       ├── request_response.go ← 请求/响应双向转换
│       └── sse.go              ← SSE chunk↔event 转换
├── zig/
│   ├── frwrapper.zig           ← @cImport llama.h + fr_load/fr_score/fr_free
│   ├── build.zig               ← zig 构建（链接 libllama）
│   └── include/                ← vendored 头文件（llama.h + ggml*.h）
├── scripts/
│   └── pack.sh                 ← 6 平台自动打包
├── fast-router.example.json    ← config 模板（无 API key）
├── go.mod / go.sum             ← Go 模块（purego v0.11.1）
└── .gitignore
```
