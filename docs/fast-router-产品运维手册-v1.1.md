---
type: Operations Manual
title: "fast-router 产品使用·配置·运维手册 v1.1"
description: fast-router 本地 LLM 智能路由网关的完整用户手册——部署、配置、API、运维、自进化周期与故障排查。对应 v1.1 功能集（级联评分/MoA/自进化/热更新/健康路由）。
timestamp: 2026-09-29
---

# fast-router 产品使用·配置·运维手册

**版本 v1.1** · 单进程本地 LLM 智能路由网关：一个端点接入，按任务智能选模、低置信自动升级多模型委员会（MoA）、越用越准（实测校准 + 可选自训练闭环）。

---

## 1. 产品概述

### 1.1 它解决什么问题

AI-agent 高频调用 LLM API 时：不同任务适合不同模型（代码/翻译/创作/分析…），手动切换低效且贵。fast-router 作为本地网关，自动完成"这个请求该发给谁"：

| 能力 | 机制 | 实测 |
|---|---|---|
| 智能选模 | 本地小模型（64M）秒级分类任务类型 → 按能力/成本/健康/实测选模 | P1 73.3%（30 样本），首评 2.77s |
| 无感 MoA | 低置信请求自动 fan-out N 个参考模型 + 聚合模型合成，客户端无感知 | 按需启用（成本闸=置信度） |
| 越用越准 | 判据回流（verdicts）→ 实测通过率自动校准选模权重 | 自动，免运维 |
| 会话继承 | 工具循环内不重复评分，续轮 ~1ms | 自动 |
| 上游健康 | 429/断连中的上游自动失去流量（有健康替代时） | 自动 |
| 自训练闭环（进阶） | 运行日志 → 教师标注 → 重训 → 回归门 → 热更新 | 需手动/脚本触发 |

### 1.2 部署形态

```
fast-router/
├── fast-router(.exe)          # Go 单二进制
├── lib/
│   ├── libfrwrapper.*         # zig 评分包装层
│   └── libllama.*/libggml.*   # llama.cpp 推理库
├── fast-router.example.json   # 配置模板
└── models/                    # GGUF 评分模型（首启可经 admin UI 下载）
```

三档部署：**hint-only**（不配模型，纯代理+协议转换）→ **fast-only**（只配小评分模型）→ **全级联**（小模型秒级首评 + 9B 后台精修）。

---

## 2. 快速开始

### 2.1 最小可用（hint-only，30 秒）

```bash
# 1) 解压分发包，编辑配置（只需填 upstreams 的 api_key）
vim fast-router.json

# 2) 启动（Unix 需指向 libllama 目录）
DYLD_LIBRARY_PATH=./lib ./fast-router          # macOS
LD_LIBRARY_PATH=./lib ./fast-router            # Linux
# Windows: 双击或 fast-router.exe（无需环境变量）

# 3) 客户端指过来
export OPENAI_BASE_URL=http://127.0.0.1:8080/v1
```

此模式下 fast-router = 多上游代理 + OpenAI↔Anthropic 双向协议转换 + 会话感知。模型名带 `上游名/` 前缀可强制指定（如 `deepseek/deepseek-chat`）。

### 2.2 启用智能路由（配置评分模型）

管理界面（`http://127.0.0.1:8080/admin`）→ Models → 选择下载（modelscope 源），或手动放置 GGUF 后配置 `model` 节。启动日志出现 `fast tier ready` 即生效。

### 2.3 首启自检清单

| 日志关键字 | 含义 |
|---|---|
| `fast tier ready (model=…)` | 小模型同步评分已启用 |
| `Jev scorer ready (model=…)` | 精修层（大模型）已加载 |
| `MoA escalation enabled` | MoA 升级已启用 |
| `no model configured — hint-only` | 未配模型（纯代理模式） |
| `warn: fast-tier model failed to load` | 小模型加载失败，首转回退 hint（检查路径/lib） |

---

## 3. 配置手册（config.json 全量参考）

```jsonc
{
  "listen": ":8080",              // 监听地址；本机使用建议 127.0.0.1:8080
  "async_score": true,            // 异步首评（缺省 true）。false 恢复同步阻塞评分（不推荐）
  "model": {
    "path": "",                   // 精修层 GGUF（如 9B）；空=不启用精修
    "fast_path": "",              // 快评层 GGUF（如 64M/0.5B）；空=首转走 hint
    "lib": "lib/libfrwrapper.dylib",
    "lib_dir": "lib"              // libllama/libggml 所在目录（DYLD/LD_LIBRARY_PATH 亦可用）
  },
  "upstreams": {                  // 真实 LLM 后端（名字自定义，registry/moa 引用）
    "openai": {
      "base_url": "https://api.openai.com/v1",  // 末段为 /vN 版本号时自动只拼方法路径
      "api_key": "sk-…",
      "protocol": "openai"        // "openai" | "anthropic"
    },
    "anthropic": { "base_url": "https://api.anthropic.com", "api_key": "…", "protocol": "anthropic" }
  },
  "registry": [],                 // 模型注册表（空=内置种子）；见 §3.2
  "moa": {                        // 低置信升级委员会（缺省关闭）
    "enabled": false,
    "min_confidence": 0.5,        // 评分置信 < 此值才升级（成本闸）
    "references": [               // 参考模型：便宜系 + 高温（多样性）
      { "upstream": "openai", "model": "openai/gpt-5-mini", "temperature": 0.6 }
    ],
    "aggregator": { "upstream": "openai", "model": "openai/gpt-5", "temperature": 0.4 }
  }
}
```

### 3.1 配置要点

- **`async_score: false` 的语义**：新任务轮同步等待评分完成（9B≈38.6s 阻塞）——仅排查用，生产保持 true。
- **fast/slow 双层**：fast 层同步出路由（64M≈2.8s）并写入会话缓存；slow 层后台精修（38.6s）覆写；续轮继承 ~1ms。**fast 层质量已接近 slow 时（同源蒸馏），可只配 fast_path 省内存**。
- **MoA 性价比原则**：N 个便宜参考 + 1 个中档聚合 ≈ 一个顶级模型的价钱、更高的一致性。工具调用请求自动跳过 MoA（聚合未证实）；参考全灭自动回退单模型。
- **多协议异构**：upstreams 可混配 openai/anthropic 协议，请求双向转换（含 SSE 流式与 tool_calls↔tool_use）。

### 3.2 registry 条目结构（自定义时）

```jsonc
[{
  "model_id": "openai/gpt-5",          // 全局唯一；上游名/模型名
  "upstream": "openai",                // 必须是 upstreams 里的键（否则被过滤）
  "display_name": "GPT-5",
  "context_window": 200000,            // 上下文门（超限请求自动跳过该模型）
  "input_cost_per_1k": 5.0,            // 冷启动选模按最便宜优先
  "output_cost_per_1k": 15.0,
  "capability_vector": { "code": "high", "reasoning": "high", "tool_use": "high", … },
  "measured": {}                       // C8 回填位（L1 自动校准使用，勿手填）
}]
```

**路由决策全输入**：任务分类（评分器）+ 能力向量（挡死）+ 成本（冷启动）+ 上下文窗口（硬门）+ 上游健康（近期错误率）+ 实测通过率（L1 自动）。

---

## 4. API 参考

### 4.1 客户端端点（与真实上游同构）

| 端点 | 协议 | 说明 |
|---|---|---|
| `POST /v1/chat/completions` | OpenAI | 含 SSE 流式（`stream:true`） |
| `POST /v1/messages` | Anthropic | 含 SSE；内部自动转换到目标上游协议 |
| `GET /v1/models` | OpenAI | 返回 `fast-router` 占位模型 |

强提示：请求 `model` 字段填 `上游名/模型id`（如 `deepseek/deepseek-chat`）可绕过评分强制指定。

### 4.2 管理/观测端点

| 端点 | 方法 | 用途 |
|---|---|---|
| `/admin` | GET | Web 管理界面（上游/注册表/模型下载/配置） |
| `/api/config` | GET/POST | 读/写配置（POST 即持久化+热生效） |
| `/api/registry` | GET/POST | 模型注册表 |
| `/api/upstream/test` | POST | 上游连通性测试 |
| `/api/models` 及 `/download` | GET/POST | 可下模型列表与异步下载（modelscope） |
| `/api/verdicts` | GET | 最近 200 条路由判据（via/task_code/outcome…） |
| `/api/measured` | GET | 实测矩阵（task×model ok_rate）+ 空层检测 |
| `/api/model/reload` | POST | **热换快评层**：`{"fast_path": "/path/new.gguf"}`，不重启 |

```bash
# 观测路由质量
curl -s localhost:8080/api/measured | jq .
# 热更新评分模型（自训练周期部署步骤）
curl -X POST localhost:8080/api/model/reload -d '{"fast_path":"models/router-v3.gguf"}'
```

---

## 5. 运维手册

### 5.1 日常

- **启动**：`DYLD_LIBRARY_PATH=./lib ./fast-router -config fast-router.json`（配置缺省时自动生成模板）
- **数据文件**（工作目录下自动产生）：
  - `data/verdicts.jsonl` — 判据回流（追加式；L1 校准+上游健康的数据源）
  - `data/train_log.jsonl` — 评分器评估记录（**含请求原文**，注意隐私；自训练教师标签源）
- **日志关键词**：`[route]` 选模 / `[route-fast]` 快评层异常 / `[route-async]` 后台精修 / `[moa]` 委员会升级 / `[hot-reload]` 热换

### 5.2 观测与调优

| 看什么 | 怎么看 | 动作 |
|---|---|---|
| 各任务类型路由质量 | `/api/measured` cells 的 ok_rate | ok_rate 低的 (task,model) 自动被 L1 降权，无需手动 |
| 任务类目空层 | `/api/measured` empty_layers | 长期空层考虑裁撤类目（C9 人工流程） |
| 上游健康 | `/api/verdicts` 里连续 upstream_error | 自动切流已生效；持续则查该上游配额/key |
| MoA 触发频率 | 日志 `[moa] escalated` | 过频→调高 min_confidence（省成本）；从不触发→检查评分器置信度分布 |
| 评分器漂移 | `train_log` 里 fast 层置信度分布变化 | 触发一次重训周期（§5.3） |

### 5.3 自训练周期（进阶运维）

```bash
# 周期性执行（建议 cron：每日/每 500 教师样本）
scripts/distill/auto_retrain.sh          # 完整管线：转换→CPU 续训→转换→回归门→热部署
# 手动分步：
python3 scripts/distill/log_to_sft.py    # train_log(教师=slow层) + 回放混合 → SFT 数据
#   训练/转换见 scripts/distill/README.md
curl -X POST .../api/model/reload -d '{"fast_path":"新模型.gguf"}'   # 热部署
```

**回归门纪律（强制）**：任何重训产物必须先过冻结基准（30 样本 P1 ≥ 当前部署值）才可 reload——脚本已内置，勿跳过（历史实测：跳门曾致质量腰斩 73.3%→46.7%）。

### 5.4 故障排查

| 症状 | 原因 | 处置 |
|---|---|---|
| 启动即退：`dlopen … libllama` 失败 | 库路径未设 | 设 `DYLD/LD_LIBRARY_PATH=lib_dir`；核对 libfrwrapper 与 libllama 同平台 |
| `fr_load returned nil` | GGUF 路径错/模型损坏/内存不足 | 核对路径；大文件重新下载；9B 需 ~7GB 空闲内存 |
| 所有请求 502 + verdicts 记 connect_error | 上游 base_url/key 错 | `/api/upstream/test` 逐个验证 |
| 上游返回 4xx 余额/鉴权 | provider 侧问题 | 换 key/充值；上游健康机制会暂时切流 |
| 首请求慢（秒级） | 快评层同步评分（正常） | 配更小的 fast 模型；或确认 `async_score:true` |
| 首请求 38s+ 阻塞 | `async_score:false` 或 fast 层加载失败 | 改回 true；查启动日志 fast-tier 告警 |
| 中文模型评分全错/报多 token | 词表无单 token 肯定词 | 已内置 yes→Yes→是 回退；自定义模型需确认三者之一是单 token |
| MoA 后回答变差 | 聚合模型太弱/参考太少 | 聚合器选强模型；参考≥2；检查 `[moa]` 日志 refs 数 |
| 长请求被拒/被截 | 上下文门生效 | 正常保护；换大窗口模型或精简请求 |

### 5.5 性能特征（本机 Intel i7 实测，供容量规划参考）

| 路径 | 延迟 |
|---|---|
| 网关热路径（继承/强提示） | ~150µs/请求 |
| 首评（新任务，64M 快评层） | ~2.8s |
| 首评（无快评层，纯 hint） | ~0ms（盲路由到默认上游） |
| 后台精修（9B，不阻塞请求） | ~38.6s |
| 热换快评层 | ~0.1-0.3s（不重启） |
| MoA 升级 | 参考并行 + 聚合（≈最慢参考 + 聚合一次） |

### 5.6 升级与回滚

- **配置回滚**：config.json 为唯一状态源（measured 校准为运行时计算，不改配置）；备份即可回滚
- **评分模型回滚**：`/api/model/reload` 指回旧 GGUF 即可（秒级）；保留上一版文件
- **程序升级**：替换单二进制重启；data/ 与配置兼容

### 5.7 安全注意事项

1. **listen 绑定**：默认 `:8080` 会监听所有网卡——本地使用改 `127.0.0.1:8080`；管理界面**不要**暴露到不可信网络（审计报告结论）
2. **API key**：明文存于 config.json，注意文件权限（`chmod 600`）
3. **隐私**：`data/train_log.jsonl` 记录请求原文（评分用）——多用户/敏感场景应评估是否保留、定期清理或禁用自训练
4. **MoA 成本**：升级路径放大 N 倍上游调用——`min_confidence` 是成本闸，监控 `[moa]` 触发频率

---

## 6. 已知限制（v1.1 诚实边界）

1. Windows 后端仅 ChoiceScore 路径（yes/no 快评层待补），真机未验证
2. MoA 对工具调用请求跳过（结构化聚合未证实）；聚合器中途失败透传给客户端（同单模型行为）
3. 评分模型质量数字（73.3%）基于 30 样本集，95% CI ±15pp；生产分布预期有偏移
4. 自训练闭环的"越用越好"尚无完整周期实证（机制就绪，见知识沉淀 v0.7）
5. 并发多会话的后台评分串行（单 context 互斥）；高并发首评可能排队

---

## 7. 相关文档

| 主题 | 路径 |
|---|---|
| 构建/打包/分发 | `SOP/build.md` |
| 架构与引擎评估 | `docs/fast-router-架构设计-v0.2.md`、`docs/fast-router-引擎评估-2026-09-27.md` |
| 自进化与证据链 | `docs/fast-router-自进化评估-2026-09-28.md`、`docs/自训练证据链深挖-2026-09-28.md` |
| MoA 研究 | `docs/MoA-研究笔记-2026-09-29.md` |
| 知识沉淀（全版本） | `docs/fast-router-知识沉淀-v0.1.md`（v0.1–v0.7） |
| 蒸馏管线 | `scripts/distill/README.md` |
