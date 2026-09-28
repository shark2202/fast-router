---
type: Research Note
title: "ai-native-theory 理论成果包研读与当前工作映射"
description: 对 ai-native-theory/（OKF 可分发理论快照）的研读，重点是新组件 KLP 知识生命周期执行包规范 v0.1 与本会话产出（沉淀 v0.5/v0.6、C7 判据、蒸馏 spike）的映射与差距。
source: ai-native-theory/ 包内 README/index/manifest/KLP 规范全文
timestamp: 2026-09-28
---

# ai-native-theory 理论成果包研读

## 结论先行（三行）

> ① 包 = OKF 格式的可分发理论快照（SHA-256 manifest、组件与 book/ 同源），**新信息是 KLP 知识生命周期执行包规范 v0.1**——它把"知识沉淀"从文档习惯升格为可移植协议：规范包+ 运行证据包（KRR）双交付物。
> ② 本会话产物与 KLP 原则**高度同构但部分符合**：双账本 ✓（patterns+DecisionRecord）、candidate/active 状态 ✓（C-001..C-011）、证据优先 ✓（来源五要素）；缺 KRR 结构化运行记录、使用事件、复验条件字段。
> ③ 采纳姿势按 KLP 自己的可逆性条款：**shadow/manual 档起步**——后续沉淀文档补三个字段（recheck_condition / use_status / 独立验证栏），不推翻现有格式。

## 一、来源

| 组件 | 状态 |
|---|---|
| README.md / index.md / manifest.json | 包身份与完整性（build_theory_dist.py 0.1.0 构建） |
| theory/ai-native组织理论.md | 理论本体（与 book/ 同源，本会话已部分使用） |
| specs/知识生命周期执行包规范-KLP.md **v0.1** | **本会话新接触**；2026-09-27 起为本项目项目级规范 |
| specs/ 其余 8 份 | 元组织/流程规范，与 book/ 同源 |

## 二、方法

通读 KLP 全文（13 节），对照本会话实际产出做映射审计；按包 README 指引"选择适用，不默认全量"。

## 三、发现：KLP 核心要求 vs 本会话现状

| KLP 要求（§2 原则/§4 状态） | 本会话产物现状 | 差距 |
|---|---|---|
| 双账本：蒸馏层 vs 回放层互不可替代 | 知识沉淀 v0.x：3.1 patterns + 3.3 DecisionRecord 校准 ✓ | 无 |
| 状态分离：lifecycle/verification/use 三维独立 | candidate/active 有（C-001..C-011）；verification 隐含（"实测/未测"混在正文）；**use_status 无** | 中 |
| 证据优先：主张可溯源 | 研究笔记五要素 + 溯源节 ✓ | 无 |
| KRR：每次实质运行独立 run_id + 结构化记录 | commit message + 沉淀文档叙事，**非结构化** | 大（但不阻塞 shadow 档） |
| 使用决定价值：使用事件登记 | C7 verdicts 就是路由决策的使用事件流（L1 已闭环）——**但知识条目本身的使用事件无** | 中 |
| 复验：recheck_condition + stale 状态 | P-021 有复验日期；其余条目无 | 中 |
| 反模式清单（§11 死法） | 本会话已规避："落盘即学习"（每次落盘附实测）、"机器绿灯即可信"（-race/构建通过≠语义正确，均补真机验证）、"验证生产同源"（I-DISC 边界在沉淀文档诚实标注未复核） | 无 |

## 四、对本项目的三个具体采纳步骤（shadow 档，低成本）

1. **沉淀条目补字段**：后续 v0.7+ 的 candidate/pattern 表增加 `recheck_condition`（何时复验）与 `use_status`（not_used/used/outcome_unknown）两列——纯 Markdown，零工具改造。
2. **KRR 轻量起步**：每个实验局（如本轮蒸馏 spike）在沉淀文档附录保留 run 块（输入/计划/验证/裁决/发布五类信息）——现有叙事已含 80%，补结构标签即可。
3. **不做的**：manifest/validators/runtime/MCP 档位——KLP 自己声明这些是完整验收（§8 冷启动/移植）的前置，当前阶段"部分符合"是诚实状态，不必伪造完整。

## 五、局限

1. 本研读未覆盖 theory/ 本体全文与 8 份 specs 的逐份差异比对（与 book/ 同源，此前已部分使用）。
2. KLP §12 自陈项目映射"部分符合"——本笔记印证该自评，未发现与其矛盾的资产。
3. 未执行包完整性校验（manifest SHA-256 逐项核对）——如需分发验收再做。

## 六、结论

理论包确认了本会话的知识工作方法在正确轨道上（原则层全对齐），差距集中在**结构化运行记录与使用事件**两个可渐进补齐的字段。按 KLP 自己的可逆性设计（shadow/manual 先行），采纳成本约等于"沉淀文档加两列表头"，无需改造现有流程。C7 verdict 系统恰好是 KLP"使用决定价值"原则在路由域的运行时实现——理论与实践在此处自然汇合。
