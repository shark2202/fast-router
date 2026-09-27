---
type: Module Guide
title: Zig FFI 与 llama.cpp 模块
description: Zig wrapper 如何隔离 llama.cpp 结构体并向 Go 暴露窄 ABI。
tags: [zig, ffi, llama-cpp, inference]
timestamp: 2026-09-26T18:00:00+08:00
---

# Zig FFI 模块

## 目标

避免 Go 直接通过 `purego` 绑定 llama.cpp 的复杂结构体。Zig 通过 `@cImport` 读取 `llama.h`，在 Zig 内创建 model/context/batch 参数；Go 只调用简单的指针、整数和数组 ABI。

## 导出函数

| 函数 | 作用 |
|---|---|
| `fr_load(path)` | 初始化 llama backend，加载 GGUF，创建 context，返回 opaque handle |
| `fr_score(handle, prompt, prompt_len, codes, n_cands, out_scores)` | tokenize、单 token 校验、decode、读取最后位置 logits（单 token 方法） |
| `fr_score_yesno(handle, prompt, prompt_len, yes_token_id, out_logit)` | 取候选 prompt 末 token 在 `yes_token_id` 上的 logit（per-candidate 方法） |
| `fr_get_token_id(handle, word, word_len)` | 解析单词（如 `"yes"`）的 token id |
| `fr_apply_template(handle, msgs_json, msgs_len, out_buf, out_buf_len, add_ass)` | 调 `llama_chat_apply_template`，按模型 chat template 渲染 |
| `fr_free(handle)` | 释放 context、model 和 handle |

## 分数路径

1. prompt tokenize 到固定缓冲区。
2. 每个候选 code tokenize，必须恰好是一个 token。
3. 候选数量不能超过 256；Jev API 约束为最多 255。
4. 清空 llama memory，避免连续调用累积 KV 状态。
5. decode prompt 并读取最后 token logits。
6. 将候选 token 的 logits 写回 Go；Go 层做 softmax。

## 构建依赖

`zig/build.zig` 链接 `libllama`、`libggml-base` 和目标平台对应的 CPU 库，
默认从 `/tmp/llama-bins/llama-b11175` 找库；可通过 `-Dllama_dir`、
`-Dggml_cpu_lib`、`-Dsysroot` 覆盖。Zig 支持 wrapper 交叉编译，但目标平台的
llama.cpp 链接库、libc 头文件和 sysroot 必须匹配；仅有运行时动态库不一定足够。

## 风险

* 固定 token/prompt 缓冲区可能限制超长输入。
* 本地 `zig/libfrwrapper.dylib` 是构建产物，不是可移植分发证明。
* `n_gpu_layers = 0` 当前 wrapper 默认 CPU；GPU 版本需要独立构建和验证（Jev 智能路由在 CPU 上 P2=77s，需 GPU/MLX 才实用）。
* Go 的 `purego` 动态库加载、共享库依赖搜索和 Zig 导出符号必须在每个平台单独验证；Windows 路径走 `syscall.NewLazyDLL`，未在 Windows 实测。
