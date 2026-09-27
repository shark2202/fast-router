# Fuck My Shit Mountain Audit Report

**Project:** fast-router
**Audit mode:** release
**Date:** 2026-09-26
**Reviewer:** OpenAI Codex

## 1. Executive Summary

**结论：当前可以作为本机/受控内测版本使用，但不建议按默认配置对外公测，更不能把管理端口暴露到不可信网络。**核心 Go 服务和路由包当前测试通过，六个平台的 Go 可执行文件也在本次审计中交叉编译成功；交付报告还记录了 hint-only 转发的 curl 端到端验证。说明项目已有可运行的原型和部分可用路径，不等于发布包、原生智能路由和完整用户旅程已达到公测门槛。

决定性阻塞是默认监听 `:8080`（所有网卡）且 `/api/config`、`/api/registry` 等管理 API 没有认证；`GET /api/config` 会返回包含上游 API key 的完整配置，未授权调用者还能修改配置。打包脚本则吞掉原生库构建/下载/提取失败后继续产包，不能证明生成的六平台压缩包能运行。报告记录四项发现：Critical 1、High 2、Medium 1。

**判断维度：**（1）用户密钥与管理面安全，（2）首次安装后核心链路可用，（3）发布构建可重复且产物真实可运行，（4）核心智能路由延迟与效果，（5）回滚和支持边界。**主要死法：**测试者启动默认配置后，同网络未授权方读取或改写密钥；或下载产物后因缺失/不匹配的原生库无法启用智能路由。**可逆性：**未发包前修复认证/绑定与打包验证成本可控；已分发后泄露的密钥必须轮换，无法靠回滚收回。

### Score Dashboard

```
Release         ███░░░░░░░  3.0  D   默认管理面无认证且监听所有网卡；原生包构建失败可被静默忽略
```

仅评估 Release 维度；该分数是发布就绪度判断，不是整体代码质量分数。

### Finding Statistics

| Severity | Count | Confirmed | Suspected |
|----------|-------|-----------|-----------|
| Critical | 1 | 1 | 0 |
| High | 2 | 2 | 0 |
| Medium | 1 | 1 | 0 |
| Low | 0 | 0 | 0 |
| Info | 0 | 0 | 0 |
| **Total** | **4** | **4** | **0** |

## 2. Project Map

- `cmd/fast-router/main.go` 是服务入口：加载 JSON 配置、按配置初始化原生 Jev scorer 或 hint-only Gateway，并把 gateway 与 admin/API 路由挂到同一 HTTP listener。
- `router/gateway.go` 接收 OpenAI/Anthropic 风格请求，选择模型并转发上游；`router/admin.go` 同时提供浏览器管理 UI 与配置、registry、模型管理 API。
- `router/config.go` 将配置写回本地文件，权限设为 `0600`；但 admin API 本身没有身份验证。
- `zig/` 提供 native wrapper；`scripts/pack.sh` 计划打出六平台 ZIP。SOP 描述了构建所需链接参数，但打包脚本没有采用这些参数，也没有验证 ZIP 内容。
- 现有交付报告和 HANDOFF 记载了模型基准、P2 延迟、历史端到端结果与尚未完成的真实 agent/Windows 验证；这些是项目自述记录，不等同于本轮复现。

### Coverage Matrix

| Dimension | Coverage | Evidence inspected | Exclusions / limits |
|-----------|----------|--------------------|---------------------|
| Release | Medium | `cmd/fast-router/main.go`、`router/admin.go`、`router/config.go`、`router/gateway.go`、`scripts/pack.sh`、`SOP/build.md`、交付报告、HANDOFF；`go test ./...`；六平台 Go 交叉编译；Git tag/CI/产物清单 | 未执行完整 pack（会写入 dist、联网下载）；未在 Windows/Linux/macOS 实机运行；未重跑 GGUF/native benchmark、真实 agent 联调、安装升级/回滚；无 CI、签名产物可供核验 |

## 3. Top Risks

1. **[Critical] 默认网络监听暴露无认证管理 API 和上游密钥**：外部可读取配置、窃取 API key，或通过写 API 改变服务配置。
2. **[High] 打包器可能把失败的原生构建当成功继续分发**：缺失 `libfrwrapper` 或 llama/ggml 依赖没有被拦截，且产物没有验收。
3. **[High] 智能路由 CPU 决策延迟约 77 秒**：项目交付报告记录的 9B CPU P2 与交互式网关公测预期不符；GPU/MLX 路径也未验证。
4. **[Medium] 没有可复核的正式发布链路与产物**：当前未见 CI、版本 tag、dist 发布包或真实跨平台冒烟验收；本次只验证 Go 可执行文件交叉编译。

## 4. Detailed Findings

### Finding: 默认监听使未认证管理 API 暴露在网络上

- Severity: Critical
- Confidence: High
- Category: Release
- Status: Confirmed
- Affected area: HTTP listener、admin UI/API、上游密钥配置
- Evidence:
  - File: `router/config.go:29-39`; `cmd/fast-router/main.go:73-90`; `router/admin.go:24-62,64-101`
  - Function / Module: `DefaultConfig`、`main`、`Admin.ServeHTTP`、`getConfig`、`postConfig`
  - Relevant behavior: 默认 `listen` 为 `:8080`；admin API 与 gateway 共用 listener。`GET /api/config` 直接 JSON 编码完整配置（含上游 API key），POST 配置/registry 无认证检查。
- Problem: 默认配置绑定所有网络接口，同时管理面没有认证或访问控制。`0600` 文件权限只保护磁盘文件，不保护通过 HTTP 暴露的管理 API。
- Why it matters: 公测用户通常会填入付费上游密钥。任何能访问该端口的同网段或转发网络调用者都可能读取密钥并修改服务配置。
- Realistic failure scenario: 用户按 README 启动服务并配置真实 API key；笔记本接入公共/办公 Wi-Fi，网络内其他主机请求 `/api/config` 获取 key，或 POST 新配置改变路由目标。
- Minimal fix: 默认绑定 `127.0.0.1:8080`；在明确配置非 loopback 监听时强制要求管理面认证，且不要通过配置读取 API 返回密钥明文（返回脱敏值）。
- Better long-term fix: 将管理面与推理 API 分离监听/权限域；建立认证、授权、CSRF/Origin 策略、密钥写入与轮换机制，并将安全默认值纳入发布验收。
- Regression test suggestion: 启动默认配置后断言只监听 loopback；测试未认证 GET/POST 管理路由被拒绝；验证读取配置响应不含原始 API key；显式公开监听时无凭证启动失败。
- Estimated effort: 1–3 days

### Finding: 原生库打包失败可被吞掉，脚本仍生成 ZIP

- Severity: High
- Confidence: High
- Category: Release
- Status: Confirmed
- Affected area: `scripts/pack.sh` 的 Zig wrapper、llama.cpp 依赖下载与 ZIP 验收
- Evidence:
  - File: `scripts/pack.sh:84-118,136-144`; `SOP/build.md:68-96`
  - Function / Module: 六平台 pack loop
  - Relevant behavior: Zig 编译命令未带 SOP 所列 `-L`、`-lllama`、`-lggml-*` 链接参数；失败用 `|| echo` 忽略。llama 下载使用 `curl -sL` 不用 `--fail`，解压和库文件复制错误被忽略；之后仍写 README 并创建 ZIP，没有检查所需动态库集合。
- Problem: 产物成功与否只以 ZIP 命令结束为准，不校验 wrapper、运行依赖、架构或启动结果。HTTP 错误响应也可能被当作下载成功处理。
- Why it matters: 发布页可能出现结构完整但无法加载原生智能路由的包；用户只能在启动/首次配置时发现问题，且六平台逐个平台排查成本高。
- Realistic failure scenario: 某目标平台 wrapper 交叉编译失败或下载包 404，脚本打印警告继续打 ZIP；用户选择该平台后 native backend 加载失败或模型调用时崩溃。
- Minimal fix: 任何预期文件缺失都令构建失败；curl 用 `--fail`；逐平台检查 wrapper 与 llama/ggml 文件存在且非空，检查动态依赖，再执行 smoke test 后才打 ZIP。
- Better long-term fix: 用可复现 CI matrix 构建每个目标平台，保存构建日志、校验和、SBOM 与签名；发布前在对应 runner/VM 解压并运行健康检查。
- Regression test suggestion: mock Zig 编译失败、HTTP 404、解压失败及缺库场景，断言脚本非零退出且不留下“成功”ZIP；正常构建时检查 ZIP manifest 和 native load smoke test。
- Estimated effort: 1–3 days

### Finding: 智能路由 CPU 延迟远超交互式使用预期

- Severity: High
- Confidence: High
- Category: Release
- Status: Confirmed
- Affected area: Jev/System One 推理、请求路由链、产品能力声明
- Evidence:
  - File: `docs/fast-router-交付报告-v1.0.md`（P1/P2 与诚实边界章节）；`HANDOFF.md`（Current state）
  - Function / Module: 原生 Jev scorer / System One 路由
  - Relevant behavior: 项目交付材料记录 Ornith-1.5-9B CPU P2 约 77 秒/决策，并注明需 GPU/MLX；hint-only 路径则绕过推理。
- Problem: 智能决策发生在转发之前，几十秒级额外等待会使常见交互请求超时或显著变慢。P1=73.3% 是小型样本集上的报告结果，不能替代延迟、稳定性及真实客户端体验验证。
- Why it matters: 若以“智能路由公测”对外宣传，用户可能把延迟归因于上游模型或认为服务卡死；启用 hint-only 虽可规避推理延迟，但不验证核心智能路由价值。
- Realistic failure scenario: 测试者在普通 CPU 设备启用 9B 模型，网关先耗时数十秒才调用云端模型，客户端在收到响应前超时/重试。
- Minimal fix: 公测默认关闭该 CPU 慢路径；明确区分 hint-only 与智能路由模式，启动时展示设备适配提示，并给出实测硬件/延迟基线。
- Better long-term fix: 先实现并验证量化/硬件加速或降低推理开销，再用固定数据集做端到端 P50/P95 benchmark 与超时预算，只有达到明确门槛后再开放智能模式。
- Regression test suggestion: 在指定最低支持硬件上测量路由 P50/P95、超时与并发；集成真实 OpenAI/Anthropic 客户端验证请求总耗时及重试行为。
- Estimated effort: 1–2 weeks（达到交互式延迟目标的优化与实测）

### Finding: 没有完整、可审计的发布验证与回滚证据

- Severity: Medium
- Confidence: High
- Category: Release
- Status: Confirmed
- Affected area: CI/CD、版本管理、安装包分发、升级/回滚说明
- Evidence:
  - File: 仓库文件清单；`scripts/pack.sh`；`docs/fast-router-交付报告-v1.0.md`；`HANDOFF.md`
  - Function / Module: Release process
  - Relevant behavior: 当前未发现 `.github` CI workflow、Git version tags 或 `dist/` 发布产物；本轮成功运行 `go test ./...`，并将 Go 主程序交叉编译到六个 OS/架构组合。交付报告也注明 Windows 实机与真实 agent 联调尚未完成。
- Problem: Go 编译通过不能证明 native wrapper、依赖库、模型下载、配置 UI、SSE/协议转换及真实客户端构成的分发包可以安装运行。没有版本化发布物和回滚指引，无法独立复核 v1.0 交付声明。
- Why it matters: 首批外部用户可能遇到只在具体 OS/架构或真实客户端触发的问题，维护者缺少稳定的已知良好版本与复现基线。
- Realistic failure scenario: beta 用户在 Windows 下载 ZIP 后 native DLL 加载失败；项目没有可定位的对应构建记录、校验和或上一版本回退说明。
- Minimal fix: 暂将公测范围限制为已实测平台/模式；生成版本化候选包与校验和，并逐包记录解压、启动、健康请求、上游转发的验收结果。
- Better long-term fix: 建立自动化 release gate：测试、六平台产物构建、包内容/依赖检查、最小端到端 smoke test、版本 changelog、签名校验和及上一版本回滚步骤。
- Regression test suggestion: 从干净环境下载候选包，按用户文档逐步安装并运行 OpenAI/Anthropic 非流式与流式请求；记录 OS、架构、配置、产物哈希和回滚结果。
- Estimated effort: 2–5 days for a scoped beta gate

## 5. Release Concerns

- **Coverage: Medium**
- **Inspected evidence:** 入口和管理面代码、配置读写、gateway 请求路径、构建脚本、SOP、交付报告、HANDOFF；`go test ./...` 通过；六平台 Go 主程序 cross-build 通过。
- **Exclusions / limits:** 未执行会联网并写 `dist/` 的完整打包流程，未在目标操作系统启动产物，未做真实 provider/agent 联调、native GGUF 推理复测、升级迁移/回滚演练或独立安全测试。交付报告中的历史基准与 E2E 仅作为待复核的项目记录。

### 发布门槛结论

| 范围 | 判断 | 依据 / 条件 |
|---|---|---|
| 本机开发/演示 | **可以，有条件** | Go tests 通过；交付报告记录 hint-only 端到端路径。仍需本机配置上游并自行验证，勿将端口暴露给不可信网络。 |
| 邀请制、隔离环境 alpha | **可考虑，仅限 hint-only 与已验证环境** | 先限制 loopback/防火墙、使用可轮换测试密钥；不要宣传 native 智能路由在普通 CPU 上可交互使用。 |
| 面向公众的 beta 下载/对外监听 | **不可以，当前阻塞** | 未先解决管理 API 无认证/默认全网监听与原生打包静默失败前，不应发布默认分发包或让用户公开端口。 |

### 必须满足的公测 Gate

1. 默认监听改为 loopback；非 loopback 监听需要认证保护管理面，配置读取脱敏；补充回归测试。
2. 修复 pack 的链接、下载、提取和内容校验；六平台原生依赖包在目标运行环境解压、启动并通过 smoke test。
3. 明确 beta 支持矩阵和模式边界；把普通 CPU 的 smart-routing 延迟现状写入面向用户的限制说明，或先达到并复测延迟目标。
4. 发布候选版本、哈希和升级/回滚说明；至少完成一轮真实客户端端到端验收并保留记录。

## 6. Recommended Fix Order

### Fix Immediately

1. 关闭默认网络管理面暴露：loopback 默认监听；公开监听需要认证。轮换任何曾在不可信网络运行过的上游密钥。
2. 修复打包静默失败，并在文件/依赖验收未通过时阻止 ZIP 产出。

### Before public beta

3. 确定 beta 的支持 OS/架构与运行模式；真实目标机器验证安装包、native backend、模型下载、两种 API 协议及流式请求。
4. 给出可观测的路由延迟指标和模式说明；未达交互预算时将智能 CPU 模式标为实验功能或关闭。
5. 固化版本号、发行说明、哈希与回滚步骤。

### Quick Wins

- 默认 `127.0.0.1:8080`，文档明确不要把管理 UI 直接暴露在公网。
- `curl --fail`；所有关键原生库构建、提取、复制操作失败即退出。
- 对 `/api/config` 的 key 做响应脱敏；测试响应中不含明文密钥。
- 将“六平台 Go 交叉编译成功”与“六平台分发包已验证”分开陈述。

## 7. 调研记录

- **来源：**当前仓库代码与配置、项目 SOP、交付报告、HANDOFF、Git 元信息。
- **方法：**优先尝试 codebase-memory MCP 图工具，但本会话未发现可用的图检索工具；按项目指引回退到定点源文件检查。运行 `go test ./...`，对 `darwin/linux/windows × amd64/arm64` 执行 `CGO_ENABLED=0 go build`，检查 tag、CI/产物清单；不运行会写入发布目录并联网下载依赖的完整 pack 脚本。
- **发现：**当前源码具备可测试的 Go 路由/网关与 hint-only 形态；默认服务配置和管理 API 不适合暴露到不可信网络；六平台 Go 编译不等价于 native 动态库 ZIP 已可分发；报告历史数据仍显示智能 CPU 路径延迟高。
- **局限：**未实测完整包、各 OS 原生 ABI、真实服务商和客户端；未重新测 P1/P2；未验证外部托管/下载流程。
- **结论：**产品已达到“本机原型/受控 alpha 可用”的阶段，不满足无额外安全约束的公开公测就绪条件。先关闭 Critical/High 项并通过真实候选包 Gate，再重新评估。
