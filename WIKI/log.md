# Wiki Update Log

## 2026-09-26（v1.0 交付后）

* **Delivery**: 同步 fast-router v1.0 交付到 Wiki；架构/引擎/方法/模型四层全部锁定，hint-only P2<1s 端到端验证、Jev 智能路由 P1=73.3%（Ornith-1.5-9B + per-candidate yes/no + apply_chat_template）。
* **Architecture**: 重写 [architecture.md](architecture.md)，补 System One Engine seam、per-candidate yes/no、apply_chat_template、session 路由缓存与 ExtendedBackend 双路径。
* **Status**: 重写 [implementation-status.md](implementation-status.md)：把 session 路由从"未闭环"上调为"测试通过 + 真实 upstream 验证"；把 Jev scorer 从"mock 测试"上调为"真实 Ornith-9B 基准 P1=73.3%"；新增 System One seam、yesnobench、6 平台 zip、schema 端到端验证；测试数从 37 校正为 60（v1.0 锁定时 39 个）。
* **Modules**: 重写 [modules/go-router.md](modules/go-router.md)、[modules/cli-and-scripts.md](modules/cli-and-scripts.md)、[modules/zig-ffi.md](modules/zig-ffi.md) 与 [build-and-test.md](build-and-test.md)：加入 ExtendedBackend 接口、yesnobench、System One Engine 两个 adapter、frwrapper 6 符号 ABI、各组测试边界与最终测试数。
* **Overview**: 重写 [project-overview.md](project-overview.md) 的成功维度、失败方式、一句话状态与结论，对齐 v1.0 基线。
* **Gaps**: 重写 [decisions/known-gaps.md](decisions/known-gaps.md)，明确标记三项已闭合（C3 真实链路、session 继承、pack.sh 验收清单），把 GPU/MLX P2、C7-C9 判据回流、I-DISC 未复核、Windows 实测、真实 agent 联调列为新缺口。
* **Docs Index**: 更新 [docs-index.md](docs-index.md)，加入交付报告 v1.0、Ornith-1.5-9B 笔记、LLM2Jev 笔记、System One 抽象笔记和审计报告。
* **Conformance**: 按 [OKF-SPEC.md](../OKF-SPEC.md) §9 / §7 自检 WIKI 知识包合规性；frontmatter 与 reserved filename 全部通过；本次将 log.md 改为 newest-first 顺序。
* **Boundary**: 未修改业务代码，未跑 Go/Zig 编译，未启动 gateway；本次仅对齐 Wiki 与 v1.0 真实状态。

## 2026-09-26

* **Research**: 新增 [Jev / System One 开源实现集成评估](../docs/Jev-开源实现集成评估.md)，区分 API 兼容、算法兼容与运行时兼容，并基于当前工作树校正旧 LLM2Jev 笔记的时效边界。
* **Research**: 复核 llm2jev、Laya、local-jev、jev-rs、openjev-sglang、AgentJev 主分支源码，记录 commit 快照、协议入口、许可证和实际运行边界。
* **Correction**: 补充核查 `jev-rs`、`laya.cpp`、`lev`，确认社区已有更贴近"GGUF + llama.cpp + 本地加速"的可集成方案，并按保留模型与优先 P2 两种目标重新排序。
* **Architecture**: 新增 [System One 引擎抽象与可替换性](../docs/System-One-引擎抽象与可替换性.md)，确认当前项目已有协议和低层 Backend 接缝，但尚未形成可插拔的完整 System One Engine。
* **Update**: 新增 [MiniCPM5-2B 深度研究](../docs/MiniCPM5-2B-深度研究.md)，记录模型事实、部署协议、论文引用链核验和 fast-router C3 适配结论。
* **Update**: 更新 [现有文档索引](docs-index.md)，纳入 MiniCPM5-2B 研究笔记。
* **Correction**: 将早期 MiniCPM5 候选笔记中的未实测速度/适配推断改为明确的待验证假设，并指向深度研究。

## 2026-09-25

* **Creation**: 建立 `WIKI/` OKF 知识包，覆盖项目概览、工作目录、架构、构建测试、模块和已知缺口。
* **Method**: 以 `AGENTS.md`、`OKF-SPEC.md`、`SOP/build.md`、Go/Zig/Python 源码、测试和现有文档交叉核对。
* **Boundary**: 未修改业务代码；未将设计文档或 Mock 实现写成生产验证结论。