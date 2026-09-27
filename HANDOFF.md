# HANDOFF — fast-router v1.0

> **Next session focus**: P2 backend amortization (prefix/KV reuse inside frwrapper to cut the 77s first score) + C7-C9 verdict feedback loop + Windows on-device test + codex/claude code real agent integration. (R5 async first-score + route cache landed 2026-09-27 — effective P2 < 1s; remaining P2 work is cutting the raw score cost.)

---

## What is fast-router

A local-first LLM smart router gateway. Clients (codex/claude code/pi-agent) send requests to one endpoint; fast-router intelligently routes to the best upstream model using Jev-style System 1 decision scoring — all in-process, no cgo, cross-platform.

**Architecture locked**: Go + purego + zig + llama.cpp + Jev system-one API + route chain (C2→C3→C4→C5) + schema conversion + admin UI + config + pack.

## Current state (v1.0 delivered)

- **P1 = 73.3%** (target >70%) — Ornith-1.5-9B + per-candidate yes/no + apply_chat_template
- **P2 = 77s raw first score** (9B CPU, hardware-bound) — **R5 async first-score + route cache landed: effective P2 < 1s** (first turn forwards on hint, background score backfills session cache, continuations inherit)
- session key now stable across tool loops (fixed v1.0 bug: growing-prefix hash never hit the cache in real agent flows)
- **hint-only mode**: fully usable (P2<1s, verified end-to-end with real LLM "pong")
- **Jev smart routing**: P1 achieved, P2 needs hardware acceleration
- 29 commits, 39 tests, 6 platforms (darwin/linux/windows × amd64/arm64)

## Key artifacts (do not duplicate — reference these)

| Artifact | Path |
|---|---|
| Delivery report | `docs/fast-router-交付报告-v1.0.md` |
| Knowledge sedimentation (v0.1+v0.2+v0.3+v0.4+v0.5) | `docs/fast-router-知识沉淀-v0.1.md` |
| Tech-stack evaluation (zig+go+cpp) | `docs/fast-router-技术栈评估-zig-go-cpp.md` |
| Engine evaluation (native/jev-rs/laya.cpp/lev) | `docs/fast-router-引擎评估-2026-09-27.md` |
| Build/pack/distribute SOP | `SOP/build.md` |
| Design consensus (grilling Q1-Q10) | `docs/fast-router-智能路由设计共识-v0.1.md` |
| Architecture design v0.2 | `docs/fast-router-架构设计-v0.2.md` |
| LLM2Jev research (per-candidate yes/no) | `docs/LLM2Jev-研究笔记.md` |
| Ornith-1.5-9B research | `docs/Ornith-1.5-9B-研究笔记.md` |
| MiniCPM5-2B research | `docs/MiniCPM5-2B-研究笔记.md` |
| fast_browser_use System 1 study | `docs/fast-browser-use-System1架构与可复用性研究.md` |
| POC progress (all P1/P2 measurements) | `docs/fast-router-POC进展-v0.1.md` |
| Go rewrite progress | `docs/fast-router-Go重写进展-v0.1.md` |

## Code structure

```
fast-router/
├── router/           # Go route chain + gateway + schema + admin
│   ├── gateway.go          # C1/C6 HTTP dual-endpoint + forward + schema convert
│   ├── scorer.go           # C3 Jev system-one scorer (Choice + yes/no + template)
│   ├── zig_backend.go      # C3 Backend (Unix: purego, ExtendedBackend)
│   ├── zig_backend_windows.go  # C3 Backend (Windows: syscall.NewLazyDLL)
│   ├── turn_detector.go    # C2 task-turn detection
│   ├── matcher.go          # C4 capability挡死
│   ├── selector.go         # C5 weighted select
│   ├── task_types.go       # 10 seed task types A-J
│   ├── registry.go        # ModelEntry + SeedRegistry
│   ├── config.go          # Config JSON persistence
│   ├── admin.go           # Admin web UI (upstreams + registry + model download)
│   ├── model_download.go  # GGUF model picker + async download
│   ├── modelscope.go      # modelscope SDK download
│   ├── errors.go          # frScoreError mapping
│   └── schema/            # OpenAI↔Anthropic conversion
│       ├── schema.go          # ConvertRequest/Response + SSEConverter
│       ├── request_response.go  # Request/response bidirectional
│       ├── sse.go             # SSE chunk↔event conversion
│       └── tool_calls.go     # Tool call conversion (tool_calls↔tool_use)
├── zig/
│   ├── frwrapper.zig      # @cImport llama.h, 6 exports (fr_load/score/score_yesno/get_token_id/apply_template/free)
│   ├── build.zig          # zig build (links libllama)
│   └── include/           # vendored headers (llama.h + ggml*.h)
├── cmd/
│   ├── fast-router/main.go  # Entry point (config + gateway + admin)
│   ├── jevbench/main.go     # Single-token P1 benchmark
│   └── yesnobench/main.go   # Per-candidate yes/no P1 benchmark
├── scripts/pack.sh       # 6-platform packager
├── SOP/build.md          # Build/pack/distribute SOP
├── fast-router.example.json  # Config template (no API keys)
└── data/task_type_samples.json  # 30 test samples (10 types × 3)
```

## Key decisions (locked)

1. **Go + CGO_ENABLED=0** — cross-compile to 6 platforms from one source
2. **purego (Unix) + syscall.NewLazyDLL (Windows)** — no cgo FFI
3. **zig @cImport llama.h** — struct ABI handled by zig compiler, Go binds narrow C ABI
4. **llama.cpp nightly prebuilt** — no compile needed, 35 platform packages
5. **per-candidate yes/no** (LLM2Jev method) — no position bias, beats single-token +6.7pp
6. **llama_chat_apply_template** — auto-adapt model's chat template (+26.6pp on 9B)
7. **Ornith-1.5-9B** — 9B Qwen3.5+ agent-trained, P1=73.3%
8. **Model not in zip** — user downloads via admin UI (modelscope SDK)

## P1 evolution (10% → 73.3%)

```
0.5B single-token:       10%
0.5B yes/no:             16.7%  (+6.7pp method)
2B yes/no:               26.7%  (+10pp model)
2B yes/no+template:      43.3%  (+16.6pp template)
9B yes/no:               46.7%  (hardcoded)
9B yes/no+template:      73.3%  (+26.6pp template) ← TARGET MET
```

Three factors by impact: **template > model size > method**

## What's NOT done (honest gaps)

1. **P2 optimization** — prefix reuse (state+instructions shared KV cache) to reduce 10x forward → ~2x. Not implemented. 9B CPU would go 77s→~15s.
2. **C7-C9 verdict feedback** — learning loop (calibrate measured_matrix from routing results). Not implemented.
3. **Windows on-device test** — compiles but never run on Windows.
4. **codex/claude code real agent integration** — all links verified via curl, but never ran a real agent client end-to-end.
5. **I-DISC** — all output by single researcher, no fresh review.
6. **GPU/MLX backend** — P2<1s needs Metal (Apple Silicon) or CUDA. zig build.zig has GPU build hooks but not tested.

## Suggested skills

- **test-driven-development** — for P2 optimization (prefix reuse) + C7-C9 implementation
- **systematic-debugging** — if P1/P2 regression or template mismatch issues arise
- **verification-before-completion** — before claiming P2<1s or C7-C9 working
- **writing-plans** — for C7-C9 verdict feedback loop design (measured_matrix calibration)
- **requesting-code-review** — to satisfy I-DISC (fresh agent reviews code + docs)

## Environment notes

- **libllama**: persistent path `~/.local/share/llama-bins/llama-b11175` (NOT /tmp, gets cleaned)
- **DYLD_LIBRARY_PATH**: `$HOME/.local/share/llama-bins/llama-b11175` (macOS)
- **Ornith GGUF**: `~/.cache/modelscope/models/ornith-ai--Ornith-1.5-9B-GGUF/snapshots/master/Ornith-1.5-9B-Q4_K_M.gguf` (5.4GB)
- **MiniCPM5-2B GGUF**: `~/.cache/modelscope/models/OpenBMB--MiniCPM5-2B-GGUF/snapshots/master/MiniCPM5-2B-Q4_K_M.gguf` (1.56GB)
- **0.5B GGUF**: `~/.cache/modelscope/models/Qwen--Qwen2.5-0.5B-Instruct-GGUF/snapshots/master/qwen2.5-0.5b-instruct-q4_k_m.gguf`
- **Build**: `go build ./...` (Go) + `cd zig && zig build-lib frwrapper.zig -dynamic -Iinclude -L"$HOME/.local/share/llama-bins/llama-b11175" -lllama -lggml-base -lggml-cpu`
- **Test**: `go test ./...`
- **Benchmark**: `DYLD_LIBRARY_PATH=... go run ./cmd/yesnobench <gguf-path>`
- **Run gateway**: `./fast-router --config fast-router.json`
- **Admin UI**: `http://localhost:8080/admin`

## Git state

- Branch: `main`
- 29 commits
- Working tree: clean
- No remote (local only, no push per AGENTS.md)
