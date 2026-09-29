# Wiki Update Log

## 2026-09-29（Kev 与 Qwen3.8 CPU 决策/推理研究）

* **Research**: 新增 [Kev System One 决策模型深度研究](../docs/Kev-System-One决策模型深度研究-2026-09-29.md)，固定 main 提交并审查其 pointer head、System One API、模型卡、服务端与研究流程。
* **Finding**: Kev 与 fast-router 已有 HTTP System One seam 协议接近，可作为 sidecar POC；通用 benchmark 不是 fast-router A–J 路由质量证据，且 Intel x64 CPU 延迟未公开验证。建议 Kev-0.8B/4B 先做 shadow，不直接替换生产引擎。
* **Research**: 新增 [Qwen3.8-27B-in-C 深度研究](../docs/Qwen3.8-27B-in-C深度研究-2026-09-29.md)，区分 CPU chat runtime 与 Jev/System One 决策引擎，并核验性能/精度边界。
* **Verification**: 在 Intel i7-9750H macOS 上，上游固定快照 `make portable` 构建及单元测试通过；`make strict` 因 `_SC_AVPHYS_PAGES` 未声明失败。未下载模型权重，未进行真实推理 benchmark。
* **Decision**: Qwen3.8 C 作为 CPU runtime 研究参考/独立本地 chat-upstream candidate；不是可直接替换 Jev 的 System One engine。此次仅新增研究文档并同步索引，未修改业务代码。

## 2026-09-29（CPU LLM 推理方案评估）

* **Research**: 新增 [CPU LLM 推理方案评估](../docs/CPU-LLM推理方案评估-2026-09-29.md)，结合 llama.cpp/OpenVINO 官方资料与本项目历史 P1/P2 实测，区分换 runtime、优化 kernel 与更换专用小模型三条路线。
* **Finding**: CPU 推理有成熟方案，但不能靠换引擎消除 9B 多候选 scoring 的计算/内存搬运成本；本地 Laya CPU 更快但 P1=56.7%，64M 专用 scorer 在 30 条集 P1=73.3% 且约 2.6s/10 前向，均需保留统计与训练数据边界。llama.cpp OpenVINO backend 可保留 GGUF，但官方构建路径为 Linux/Windows，验证重点 Core Ultra 1/2，Qwen3.5 9B CPU stateful 未通过，量化准确率/性能仍在验证中。
* **Recommendation**: 保留 9B 质量基线；先测试 64M fast tier + shadow、llama.cpp 参数 sweep、KV/prompt 复用；OpenVINO 只能在 Linux/Windows Intel 环境做 stateless/目标场景对照，不能直接替换当前批量 KV scorer；交互延迟要求用已验证的异步首评，不能承诺通用 9B CPU 同步亚秒。

## 2026-09-28（Jev 路由决策效果评估）

* **Research**: 新增 [Jev 路由决策效果评估](../docs/Jev路由决策效果评估-2026-09-28.md)，明确当前 `verdicts.jsonl` 只能证明 HTTP/上游运行结果，不能直接证明 Jev task code 正确或选模优于候选。
* **Model**: 将评估拆成采集层、运行层、决策层、因果层；补充 `inherit` 去重、错误分类、反馈偏置和 shadow/A-B 方案。
* **Boundary**: 当前 `ok_rate` 降级为运行代理指标；`train_log` 只作为 fast/slow 漂移与分歧数据，不作为外部真值。
* **Next**: P0 增加 request/decision 关联、延迟、版本、token、error_class；P1 离线报表；P2 独立 task label 与 baseline；P3 session-level shadow/A-B。

## 2026-09-28（离线六平台交叉构建 POC）

* **Knowledge**: 将六平台交叉构建、离线本地归档和 macOS x64 启动冒烟沉淀为 `docs/fast-router-知识沉淀-v0.1.md` v0.7，按来源/方法/发现/局限/结论记录，并新增 P-018～P-021、L-018～L-021、C-012～C-014。
* **Build**: 更新 [build-and-test.md](build-and-test.md)，明确 `OFFLINE=1 + LLAMA_ARCHIVE_DIR`、Windows GNU import library、构建级与运行级边界。
* **Status**: 更新 [implementation-status.md](implementation-status.md)，把六平台状态校正为“macOS x64 主机构建 POC”，只把 macOS x64 hint-only 启动列为运行证据。
* **Evidence**: 六个目标 ZIP 和离线六目标 ZIP 完整性检查通过；macOS x64 包通过 `/api/config`、`/admin`、`/v1/models`。Linux/Windows runtime、GGUF/native scorer 仍未验收。
* **Boundary**: 本次只沉淀知识和同步 Wiki，未修改业务逻辑；新 POC 条目保持 candidate，需第二次独立复现或新鲜评审后升级。

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
