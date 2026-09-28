---
type: Build and Test
title: fast-router 构建与测试指南
description: 从本地测试到六平台打包的操作路径，以及每一步的验证边界。
tags: [build, test, release, operations, v1.0]
timestamp: 2026-09-28T18:00:00+08:00
---

# 构建与测试

## 前置依赖

| 工具 | SOP 要求 | 用途 |
|---|---:|---|
| Go | >= 1.25 | CGO_ENABLED=0 编译 Go |
| Zig | 0.14.x | 编译 `libfrwrapper` 和目标平台 wrapper |
| curl | 在线模式需要 | 下载 llama.cpp nightly；离线模式不需要 |
| zip | 任意 | 生成分发包 |
| tar/unzip/objdump | 任意 | 解包归档、检查 ZIP、生成 Windows 导出列表 |
| Python | 非 Go 构建必需 | 模型下载路径可能调用 modelscope |

## 本地 Go 验证

```bash
gofmt -l cmd router

go test ./...
go build ./cmd/fast-router
```

完成标准：`gofmt -l` 无输出；`go test ./...` 返回 0；server binary 编译成功。测试通过只证明 Go 逻辑和测试替身，不证明动态库/模型链路。

## Zig wrapper 验证

```bash
zig version
zig build -Doptimize=ReleaseFast -Dllama_dir=/path/to/llama-bins
```

完成标准是生成目标平台共享库，并能解析 `fr_load`、`fr_score`、`fr_free` 所需的 llama.cpp 依赖。目标平台构建通过 `zig build -Dtarget=...` 完成；Windows 当前使用 `windows-gnu`，并从 DLL 导出生成 GNU import library。

## 真实 scorer/benchmark

需要 GGUF、`libfrwrapper`、`libllama` 和平台库路径：

```bash
# 单 token Choice 打分（历史方法，对照基线）
DYLD_LIBRARY_PATH=/path/to/llama-libs \
  go run ./cmd/jevbench /path/to/model.gguf

# per-candidate yes/no + apply_chat_template（v1.0 主方法，P1=73.3% 主验收路径）
DYLD_LIBRARY_PATH=/path/to/llama-libs \
  go run ./cmd/yesnobench /path/to/model.gguf

# 单样本 Zig/FFI 调试
go run ./cmd/zigbench /path/to/model.gguf
```

Windows/Linux 使用对应的库路径变量。benchmark 结果应单独记录准确率、延迟、模型、平台和库版本；不能把结果写成泛化结论。任何 P1/P2 数字必须能从 `yesnobench` / `jevbench` 实际跑出。

## 启动 server

```bash
CGO_ENABLED=0 go build -o fast-router ./cmd/fast-router
DYLD_LIBRARY_PATH=/path/to/llama-libs \
  ./fast-router --config ./fast-router.json
```

`fast-router.json` 不应提交，因为包含 API key；可以从 `fast-router.example.json` 复制。没有 `model.path` 时是 hint-only 模式；配置 GGUF 路径后才尝试初始化 Jev backend。

## 六平台打包

```bash
# 在线准备/构建
VERSION=0.1.0 LLAMA_VER=b11175 ./scripts/pack.sh

# 使用已准备好的六份本地归档，禁止网络访问
OFFLINE=1 \
LLAMA_ARCHIVE_DIR=/path/to/llama-archives \
PLATFORM_LIST='darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64' \
./scripts/pack.sh
```

脚本目标是 `darwin/{amd64,arm64}`、`linux/{amd64,arm64}`、`windows/{amd64,arm64}`。单一 macOS x64 主机已完成六目标 ZIP 构建 POC，且离线模式已用本地归档复现；这只证明构建级和包级，不证明 Linux/Windows 运行级可用。脚本遇到缺失归档、工具、符号或链接错误会失败退出，不应继续发布残包。

当前运行证据只有 macOS x64 hint-only ZIP：解压后启动并通过 `/api/config`、`/admin`、`/v1/models`；未加载 GGUF。六平台正式发布仍需逐目标 native 库加载、最小 API 和模型 smoke。

## 用户侧验收

1. 解压包并设置平台库路径。
2. 启动 server，确认 `/admin` 可访问。
3. `GET /api/config` 返回 JSON。
4. 配置 upstream，调用 `/api/upstream/test`。
5. 下载或配置 GGUF，确认 `/api/models/download/status` 完成。
6. 用 OpenAI 和 Anthropic 请求分别验证非流式、流式、工具结果和跨协议转换。

## 故障优先级

* 动态库找不到：先检查 `model.lib` 和 `DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH`。
* wrapper 链接失败：检查 `zig/include` 与 llama.cpp 库目录。
* 模型下载失败：检查 modelscope SDK、网络和文件名。
* 404：检查 server 进程、端口和 mux 路由。
* SSE 错误：先用 curl 保留原始事件，再检查 `SSEConverter` 状态机。
