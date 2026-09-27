---
type: Research Note
title: "引擎评估：native zig/llama.cpp vs jev-rs / laya.cpp / lev / llm2jev"
description: 基于本机实测核验（硬件、llama.cpp 包内容、四仓库活跃度、llama-server 端点能力）的引擎路线决策评估，按维度-死法-可逆性框架。
source: 本机核验 + docs/Jev-开源实现集成评估.md + GitHub API
timestamp: 2026-09-27
---

# 引擎评估（2026-09-27）

## 结论先行（三行）

> ① **在这台 Intel Mac 上，换任何引擎都解不开 P2**——x64 预编译包无 Metal 后端（实测），所有 sidecar 路线的推理仍跑 CPU；P2<1s 的正解是**架构（异步首评+路由缓存）而非引擎**。
> ② 引擎对照的第一条实验线是 **jev-rs sidecar**（GGUF/模型保留 + llama-server `cache_prompt` 默认开启白得前缀 KV 复用），laya.cpp 是唯一可能原生 <1s 的路线但**必须换模型 → P1 重验**，只做 spike。
> ③ native 路径保持默认不动（P1=73.3% 已达标、分发最简、可逆性最高）。

---

## 一、来源

| 来源 | 内容 | 核验方式 |
|---|---|---|
| 本机硬件 | i7-9750H + AMD Radeon Pro 5300M 4GB（Metal 3）+ Intel UHD 630 | `system_profiler` 实测（2026-09-27） |
| llama.cpp b11175 macos-x64/arm64 包 | x64 **无** libggml-metal；arm64 **有**；两包均含 llama-server | 下载解包实测（2026-09-27） |
| llama-server b11175 能力 | `cache_prompt` 默认 true；`n_probs` 可取 token 概率；**无**多候选单 forward 打分端点 | binary `--help` + 官方 README@b11175 |
| 四仓库状态 | 见 §3.1 表 | GitHub API（2026-09-27） |
| 社区项目能力边界 | laya.cpp 需 Laya checkpoint、jev-rs 依赖 llama-server 等 | `docs/Jev-开源实现集成评估.md`（2026-09-26 快照，本轮抽查未推翻） |
| 本仓接缝现状 | SystemOneEngine + Native/HTTP 双实现（7d41efe） | `router/systemone_engine.go` / `systemone_http.go` |
| 现有缺口清单 | yes/no 路径缺陷（Instructions 未用、无 P(yes|yes,no) 归一化等） | Jev 评估文档"待核实点"节 |

## 二、方法

按维度/死法/可逆性评估六条路线。关键方法约束：**不依据 README 自报 benchmark 排名**（Jev 评估文档已确立此纪律）；硬件事实以本机实测为准；包能力以解包为准。

## 三、发现

### 3.1 核验事实（全部当日实测/实查）

| # | 事实 | 含义 |
|---|---|---|
| F1 | 本机 x86_64，GPU=AMD 5300M 4GB（Metal 3） | 唯一 GPU 路径是 Metal@AMD |
| F2 | b11175 x64 包只含 base/cpu/blas/rpc 后端，**无 libggml-metal**；arm64 包有 | **官方预编译在这台机器上 GPU=0**；要用 Metal 必须自编译（打破"预编译分发"不变量，SOP ❌ C 编译器条款） |
| F3 | `n_gpu_layers = 0` 硬编码于 frwrapper.zig:23 | 即使有 Metal 后端也未启用（当前是有意无意地与 F2 自洽） |
| F4 | llama-server `cache_prompt` 默认 true（跨请求前缀 KV 复用）；`n_probs` 可读 token 概率；无 /score 类多候选单 forward 端点 | 子进程路线**免费获得** HANDOFF 计划的"P2 prefix reuse"；打分需 N 次请求但前缀复用可摊销；**C-008 candidate 部分解除** |
| F5 | jev-rs：Apache-2.0，今日仍推送，12★，`jev serve` /v1/systemone，依赖 llama-server，任意 GGUF | GGUF 保留路线的服务化选项，活跃 |
| F6 | laya.cpp：MIT，昨日推送，95★（四者最热），CoreML/Metal/CUDA/Vulkan，**需 Laya checkpoint（非任意 GGUF）** | 唯一可能原生 <1s 的本地路线，但模型必须换 |
| F7 | lev：Apache-2.0，3 天前推送，22★，Jolt/Clojure runtime | runtime 重量与 Go 网关 6 平台 zip 目标冲突 |
| F8 | llm2jev：MIT，3 天前推送，5★，当前法=single prefill label logits；其 MLX 后端仅 Apple Silicon | 在本机（Intel）无加速后端，同 CPU 瓶颈 |
| F9 | 本仓 SystemOneEngine seam 已落地（Native + HTTP 双实现） | 任何 /v1/systemone sidecar 可配置接入，shadow 可行 |

### 3.2 六条路线评估

#### R0 native（zig/llama.cpp 进程内，现状）——**保持默认**
- **维度**：P1=73.3% ✅；P2=77s（同步首评）；6 平台分发 ✅；单进程零依赖 ✅。
- **死法**：本机 P2 硬件死结（F2：无 GPU 后端可用）；zig/llama 版本耦合（已知持续成本）。
- **可逆性**：最高（Backend 接口 + hint-only 兜底）。
- **动作**：不动引擎；P2 靠 R5。

#### R0.5 自编译 llama.cpp + Metal@AMD——**否决（本轮）**
- **维度**：推测 77s→15~30s（部分卸载；9B Q4≈5.4GB > 4GB VRAM）；**<1s 不可达**。
- **死法**：打破预编译不变量 → 每平台自建编译矩阵，发布复杂度爆炸；AMD GPU 的 llama.cpp Metal 支持质量待实测（推断，非确证）。
- **可逆性**：中（构建侧）。
- **判定**：收益/成本比最差，先不做。

#### R1 jev-rs sidecar——**第一条对照实验线**
- **维度**：GGUF/模型保留（P1 理论可保持，**须实测**——Jev 文档警告 prompt/logits 数学细节不一致风险）；`/v1/systemone` 直配已有 seam（F9）；llama-server `cache_prompt` 白得前缀 KV 复用（F4）→ 跨候选打分摊销；Rust 二进制分发可控。
- **死法**：两额外进程（llama-server + jev serve）；本机推理仍 CPU → P2 绝对值不降；wire 通 ≠ 算法同（三种兼容混淆，Jev 文档已立此纪律）。
- **可逆性**：高——配置开关 + shadow 模式，native 保持默认。
- **动作**：shadow 对照（冻结样本集，测 P1 一致性 + 摊销 P2）。

#### R2 llm2jev sidecar——**本机不优先**
- F8：本机无 MLX → CPU transformers，P2 更差。Apple Silicon 设备上再评。

#### R3 laya.cpp——**spike only（watchlist）**
- **维度**：专用 decision head（小模型）→ **唯一可能真 <1s 的本地路线**；MIT；社区最热（F6）。
- **死法**：**必须换模型 → P1=73.3% 作废全量重验**；Laya checkpoint 生态是否覆盖 agent 路由任务未知；Intel x86_64 Mac 支持边缘。
- **可逆性**：高（HTTP sidecar）。
- **动作**：一次性 P1 快测 spike——仅当其 checkpoint 目录存在 agent/coding/routing 类目时升级为正式对照线。

#### R4 lev——**否决**
- F7：Clojure/Jolt runtime 与"Go 单进程 + 6 平台 zip"发行形态冲突。留作参考实现阅读。

#### R5 架构解：异步首评 + 路由缓存——**P2 的真正答案**
- **洞察**：P2=77s 是**同步首评**延迟；本仓 C2 任务轮检测 + session 路由继承已实现"score once, reuse"。把首评异步化（首轮按 hint 转发，评分完成后供后续轮），有效 P2≈0 且 **P1 不变**。
- **死法**：首轮决策质量（hint 兜底已验证可用）；任务切换瞬间的首评空窗（C2 检测决定何时需新评）。
- **可逆性**：高（纯 Go 路由链改动，不碰引擎）。
- **动作**：与 HANDOFF"P2 optimization (prefix reuse)"合并设计——进程内做 frwrapper 级 KV 复用（llama memory 序列 API），或直接借 R1 的 llama-server 路线白得。

### 3.3 决策表

| 路线 | P1 风险 | P2 预期（本机） | 分发影响 | 可逆性 | 判定 |
|---|---|---|---|---|---|
| R0 native 现状 | 无（73.3% 已锁） | 77s | 无 | 最高 | **默认保留** |
| R0.5 自编译 Metal | 无 | 15~30s（推测） | +编译矩阵 | 中 | 否决（暂） |
| R1 jev-rs sidecar | 需实测一致性 | 摊销后显著降 | +2 进程 | 高（seam 已在） | **对照线①** |
| R2 llm2jev | 中 | 更差（本机 CPU） | +Python | 高 | 本机不优先 |
| R3 laya.cpp | **高（换模型全量重验）** | 可能 <1s | +转换链 | 高 | spike only |
| R4 lev | 中 | 不明 | +Clojure runtime | 中 | 否决 |
| R5 异步+缓存 | 无 | **有效≈0** | 无 | 高 | **与 R0 合并执行** |

## 四、局限

1. R0.5 的 Metal@AMD 性能与 R3 的 Laya checkpoint 类目均未实测（分别标"推测"与"待查"）。
2. R1 的 P1 一致性未验证——jev-rs 的 prompt/logits 数学与 fast-router yes/no 路径不同（yes token logit + 候选维 softmax vs 其自有实现），shadow 对照是解除条件。
3. 本机=Intel Mac 的结论不可外推到用户实际部署设备（若目标设备是 Apple Silicon/arm64 或有 N 卡，F2/F8 结论反转，需重评）。
4. llama.cpp 端点能力仅核验 b11175 单版本；上游演进快（b-series 每日构建）。
5. 社区项目能力边界沿用 2026-09-26 快照 + 本轮活跃度抽查，未重读全部 README 细节。

## 五、结论

**"评估引擎"的正确输出不是"换哪个引擎"，而是分层裁决：引擎层 R0 保持默认（P1 与分发优势压倒一切）；P2 的解在架构层（R5 异步首评+缓存，P1 零风险）；对照实验层 R1 jev-rs 是第一条线（GGUF 保留 + 白得 KV 复用 + seam 已就绪）；R3 laya.cpp 是唯一 <1s 替代可能但被"换模型→重验 P1"挡在 spike 门内；R0.5/R2/R4 本轮否决。** 所有判定随约束（目标设备硬件）变化需重跑——特别是 F2 若在 arm64 设备上反转，R0.5 与 R2 立即复活。

## Citations

1. llama.cpp b11175 macos-x64 / macos-arm64 包（2026-09-27 下载解包实测，/tmp/llama-verify）
2. llama-server 0.5.0-dev build 11175 `--help` 与 tools/server/README.md@b11175
3. https://github.com/yijunyu/jev-rs（GitHub API，pushed 2026-09-27）
4. https://github.com/lkarlslund/laya.cpp（pushed 2026-09-26）
5. https://github.com/jlt-commons/lev（pushed 2026-09-24）
6. https://github.com/tic-top/llm2jev（pushed 2026-09-24）
7. `docs/Jev-开源实现集成评估.md`（2026-09-26，9 项目快照 + 三种兼容框架）
8. `router/systemone_engine.go` / `router/systemone_http.go`（seam 实体）
