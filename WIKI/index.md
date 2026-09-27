# fast-router 项目 Wiki

> OKF v0.1 knowledge bundle。这里是项目工作目录、当前实现、构建流程和已知边界的渐进式入口。
>
> **当前基线**：fast-router v1.0 已交付（2026-09-26）。架构/引擎/方法/模型四层全部锁定：
> hint-only P2<1s 端到端可用；Jev 智能路由 P1=73.3%（Ornith-1.5-9B + per-candidate yes/no + apply_chat_template）。
> 详细决策见 [交付报告 v1.0](../docs/fast-router-交付报告-v1.0.md)。剩余缺口见 [已知缺口与风险](decisions/known-gaps.md)。

## 先读

* [项目概览](project-overview.md) - 项目目的、事实来源、成功维度、失败模式与可逆性
* [工作目录地图](workspace-map.md) - 整个仓库的目录和文件职责
* [架构与数据流](architecture.md) - C1-C6 路由链、System One Engine seam、Go/Zig/Python 边界和请求流
* [实现状态](implementation-status.md) - v1.0 后的状态矩阵：代码存在 / 测试通过 / 端到端验证 / 设计规划
* [构建与测试](build-and-test.md) - `SOP/build.md` 的可执行摘要和验证命令

## 开发者入口

* [开发协作指南](developer-guide.md) - AGENTS、OKF、Git 和变更边界
* [Go Router 模块](modules/go-router.md) - 当前主实现的模块职责与接口
* [Zig FFI 模块](modules/zig-ffi.md) - `frwrapper` 与 llama.cpp 边界
* [Python POC 模块](modules/python-poc.md) - 历史验证实现和它的适用边界
* [脚本与命令入口](modules/cli-and-scripts.md) - server、benchmark、pack 等入口

## 知识索引

* [现有文档索引](docs-index.md) - `docs/`、`book/`、`data/`、`SOP/` 的导航
* [已知缺口与风险](decisions/known-gaps.md) - 代码与文档中明确的未闭合项

## 使用规则

1. 先看 Wiki，再看代码；Wiki 只描述已从仓库事实核验的内容。
2. “已实现”不等于“已在真实模型、真实 upstream 或六平台上验证”。
3. 当 Wiki 与代码冲突时，以代码、测试和 `SOP/build.md` 的当前命令为准，并更新本目录。
