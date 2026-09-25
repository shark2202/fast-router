---
type: Build and Test
title: fast-router 构建与测试指南
description: 从本地测试到六平台打包的操作路径，以及每一步的验证边界。
tags: [build, test, release, operations]
timestamp: 2026-09-25T00:00:00+08:00
---

# 构建与测试

## 前置依赖

| 工具 | SOP 要求 | 用途 |
|---|---:|---|
| Go | >= 1.25 | CGO_ENABLED=0 编译 Go |
| Zig | >= 0.14 | 编译 `libfrwrapper` |
| curl | 任意 | 下载 llama.cpp nightly |
| zip | 任意 | 生成分发包 |
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

也可以按 `SOP/build.md` 使用 `zig build-lib`。完成标准是生成目标平台共享库，并能解析 `fr_load`、`fr_score`、`fr_free` 所需的 llama.cpp 依赖。

## 真实 scorer/benchmark

需要 GGUF、`libfrwrapper`、`libllama` 和平台库路径：

```bash
DYLD_LIBRARY_PATH=/path/to/llama-libs \
  go run ./cmd/jevbench /path/to/model.gguf
```

Windows/Linux 使用对应的库路径变量。benchmark 结果应单独记录准确率、延迟、模型、平台和库版本；不能把结果写成泛化结论。

## 启动 server

```bash
CGO_ENABLED=0 go build -o fast-router ./cmd/fast-router
DYLD_LIBRARY_PATH=/path/to/llama-libs \
  ./fast-router --config ./fast-router.json
```

`fast-router.json` 不应提交，因为包含 API key；可以从 `fast-router.example.json` 复制。没有 `model.path` 时是 hint-only 模式；配置 GGUF 路径后才尝试初始化 Jev backend。

## 六平台打包

```bash
VERSION=0.1.0 LLAMA_VER=b11175 ./scripts/pack.sh
```

脚本目标是 `darwin/{amd64,arm64}`、`linux/{amd64,arm64}`、`windows/{amd64,arm64}`。它会编译 Go、尝试编译 Zig、下载 llama.cpp nightly、组装 zip。任何一个目标的下载或 Zig 交叉编译失败，都可能留下不完整包；打包后必须逐包检查内容和启动。

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
