---
type: Workspace Map
title: fast-router 工作目录地图
description: 仓库顶层目录、文件职责和生成物边界。
tags: [workspace, files, onboarding]
timestamp: 2026-09-25T00:00:00+08:00
---

# 工作目录地图

## 顶层结构

```text
fast-router/
├── AGENTS.md                    # Agent 协作与决策约束
├── OKF-SPEC.md                  # OKF 知识包规范
├── SOP/build.md                 # 构建·打包·分发 SOP
├── WIKI/                        # 本项目知识 Wiki
├── cmd/                         # Go 可执行入口
├── router/                      # Go 路由核心、网关、管理 API、协议转换
├── zig/                         # llama.cpp FFI wrapper 和 vendored headers
├── scripts/                     # benchmark、跨平台打包脚本
├── fast-router.example.json     # 无密钥配置模板
├── go.mod / go.sum              # Go 模块和依赖
├── src/fast_router/             # Python POC 包
├── tests/                       # Python POC 测试
├── data/                        # POC 样例数据
├── docs/                        # 研究、设计、进展文档
└── book/                        # ai-native 理论和组织流程知识库
```

## Go 主实现

### `cmd/`

* `fast-router/main.go`：读取配置，按是否存在 `model.path` 初始化 hint-only 或 Jev 模式，启动 `/v1/*`、`/admin` 和 `/api/*`。
* `jevbench/main.go`：真实模型/Backend benchmark 入口。
* `zigbench/main.go`：单样本 Zig/FFI 调试入口。

### `router/`

* `gateway.go`：OpenAI/Anthropic 端点、路由链、upstream 转发和 SSE/响应转换。
* `scorer.go`：Jev system-one Choice API 结构、候选编码、Backend 接口和 softmax。
* `turn_detector.go`：任务轮、工具循环和 ambiguous 判定。
* `matcher.go`：按 capability vector 挡死模型。
* `selector.go`：按实测 pass rate 与成本选择模型。
* `task_types.go`：10 个种子任务类型及能力需求。
* `registry.go`：模型注册项、成本、能力向量和 measured 数据。
* `config.go`：JSON 配置加载、保存和 upstream 转换。
* `admin.go`：HTML 管理页与配置/registry/upstream API。
* `model_download.go`、`modelscope.go`：模型列表与异步下载路径。
* `errors.go`、`zig_backend*.go`：错误映射和跨平台 FFI 后端。
* `schema/`：OpenAI ↔ Anthropic 请求、响应和 SSE 转换。

### `router/*_test.go`

Go 测试覆盖四组：C2 任务轮判定、C3 scorer、C4/C5 matcher-selector、schema 请求/响应/SSE 转换。

## Zig FFI

* `zig/frwrapper.zig`：通过 `@cImport("llama.h")` 隐藏 llama.cpp 结构体，导出 `fr_load`、`fr_score`、`fr_free`。
* `zig/build.zig`：构建共享库并链接 `libllama`、`libggml-base`、`libggml-cpu`。
* `zig/include/`：vendored llama.cpp/ggml 头文件。
* `zig/*.dylib`、`zig/*.o`、`.zig-cache/`：本地构建生成物，已由 `.gitignore` 排除，不是 Wiki 的源代码边界。

## Python POC

* `src/fast_router/jev_scorer.py`：早期 Jev 单 token 打分实现。
* `src/fast_router/turn_detector.py`、`matcher.py`、`selector.py` 等：Python 版逻辑 POC。
* `tests/`：Python 版任务轮和 matcher/selector 测试。
* `scripts/benchmark_jev.py`：Python benchmark。

## 知识与输入数据

* `docs/`：研究、设计共识、架构设计和阶段性进展。
* `book/`：ai-native 组织、工程、测试、需求与知识沉淀理论。
* `data/task_type_samples.json`：任务类型样例。

## 不应进入版本库的生成物

`.gitignore` 排除了 Python cache、egg-info、Go binary/test artifacts、Zig build cache/shared libraries、模型权重、运行配置和系统文件。`fast-router.example.json` 是唯一明确保留的配置模板。
