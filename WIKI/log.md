# Wiki Update Log

## 2026-09-25

* **Creation**: 建立 `WIKI/` OKF 知识包，覆盖项目概览、工作目录、架构、构建测试、模块和已知缺口。
* **Method**: 以 `AGENTS.md`、`OKF-SPEC.md`、`SOP/build.md`、Go/Zig/Python 源码、测试和现有文档交叉核对。
* **Boundary**: 未修改业务代码；未将设计文档或 Mock 实现写成生产验证结论。

## 2026-09-26

* **Research**: 新增 [Jev / System One 开源实现集成评估](../docs/Jev-开源实现集成评估.md)，区分 API 兼容、算法兼容与运行时兼容，并基于当前工作树校正旧 LLM2Jev 笔记的时效边界。
* **Research**: 复核 llm2jev、Laya、local-jev、jev-rs、openjev-sglang、AgentJev 主分支源码，记录 commit 快照、协议入口、许可证和实际运行边界。
* **Update**: 新增 [MiniCPM5-2B 深度研究](../docs/MiniCPM5-2B-深度研究.md)，记录模型事实、部署协议、论文引用链核验和 fast-router C3 适配结论。
* **Update**: 更新 [现有文档索引](docs-index.md)，纳入 MiniCPM5-2B 研究笔记。
* **Correction**: 将早期 MiniCPM5 候选笔记中的未实测速度/适配推断改为明确的待验证假设，并指向深度研究。
