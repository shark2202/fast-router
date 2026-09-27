---
type: Module Guide
title: 命令与脚本入口
description: fast-router 的 server、benchmark、打包和辅助脚本入口。
tags: [cli, scripts, packaging]
timestamp: 2026-09-25T00:00:00+08:00
---

# 命令与脚本入口

## Server

```bash
go run ./cmd/fast-router --config ./fast-router.json
```

默认配置路径依次受 `--config`、`FR_CONFIG`、`./fast-router.json` 影响。配置不存在时会尝试生成默认模板。`model.path` 为空则 hint-only；有路径则尝试初始化 Zig backend。

## Benchmark

```bash
# 单 token Choice 打分（历史方法，对照基线）
go run ./cmd/jevbench /path/to/model.gguf

# per-candidate yes/no + apply_chat_template（v1.0 主方法）
go run ./cmd/yesnobench /path/to/model.gguf

# 单样本 Zig/FFI 调试
go run ./cmd/zigbench /path/to/model.gguf
```

三者都依赖真实 GGUF/动态库环境。`yesnobench` 是 P1=73.3% 的主验收入口；`jevbench` 保留作单 token 方法对照；`zigbench` 用于单样本 FFI 调试。

真实模型的 P1/P2 结果不能从 mock 测试推断；任何基准数字必须从 `yesnobench` / `jevbench` 实际跑出。

## 打包

```bash
./scripts/pack.sh
```

可用 `VERSION`、`OUTDIR`、`LLAMA_VER`、`ZIG_BIN`、`PLATFORM_LIST`、`OFFLINE`、
`LLAMA_ARCHIVE_DIR` 以及可选的
`ZIG_SYSROOT_<OS>_<ARCH>` 环境变量覆盖默认值。默认生成 `./dist/` 下的六平台
zip，目标平台为 macOS、Linux、Windows 的 amd64/arm64。脚本会在网络、Zig、
目标库、Windows import library 或 wrapper 产物缺失时失败。必须运行跨平台验收
清单，不能仅以 Go 交叉编译作为发布证明。

设置 `OFFLINE=1` 后，脚本只从 `LLAMA_ARCHIVE_DIR` 读取六个平台的 llama.cpp
归档，不调用 GitHub 或 `curl`；适合内网/纯本地构建。

## Python benchmark

```bash
PYTHONPATH=src python scripts/benchmark_jev.py
```

这是历史 POC 路径，不能替代 Go 主线 benchmark。
