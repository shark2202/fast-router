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
go run ./cmd/jevbench /path/to/model.gguf
go run ./cmd/zigbench /path/to/model.gguf
```

两者依赖真实 GGUF/动态库环境；用途分别是批量 Jev 任务样例和单样本 FFI 调试。

## 打包

```bash
./scripts/pack.sh
```

可用 `VERSION`、`OUTDIR`、`LLAMA_VER` 环境变量覆盖默认值。脚本会生成 `dist/` 下的六平台 zip，且可能因为网络、Zig 或平台库问题生成不完整包；必须运行 SOP 验收清单。

## Python benchmark

```bash
PYTHONPATH=src python scripts/benchmark_jev.py
```

这是历史 POC 路径，不能替代 Go 主线 benchmark。
