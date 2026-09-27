// frwrapper.zig — narrow C ABI wrapper over llama.cpp for Jev system-one scoring.
//
// Two scoring modes:
//   fr_score          — single-token multi-candidate (original fast_browser_use method)
//   fr_score_yesno     — per-candidate independent yes/no (LLM2Jev method, no position bias)
//   fr_apply_template  — llama_chat_apply_template (auto-adapt Qwen3.5/Ornith, no hardcoded format)
//
// Hides llama_batch / model_params / context_params structs (handled by zig
// via @cImport) and exposes only scalar+pointer functions for purego to bind.
const std = @import("std");
const c = @cImport({
    @cInclude("llama.h");
});

const Handle = extern struct {
    model: ?*c.llama_model,
    ctx: ?*c.llama_context,
};

export fn fr_load(model_path: [*:0]const u8) ?*Handle {
    c.llama_backend_init();
    var mp = c.llama_model_default_params();
    mp.n_gpu_layers = 0;
    const model = c.llama_load_model_from_file(model_path, mp);
    if (model == null) return null;
    var cp = c.llama_context_default_params();
    cp.n_ctx = 8192;
    cp.n_batch = 8192;
    const ctx = c.llama_new_context_with_model(model, cp);
    if (ctx == null) {
        c.llama_free_model(model);
        return null;
    }
    const h: *Handle = @ptrCast(@alignCast(std.c.malloc(@sizeOf(Handle)) orelse return null));
    h.* = .{ .model = model, .ctx = ctx };
    return h;
}

// --- Original method: single-token multi-candidate (fast_browser_use) ---

export fn fr_score(
    h: ?*Handle,
    prompt: [*]const u8,
    prompt_len: usize,
    cand_codes: [*]const [*:0]const u8,
    n_cands: c_int,
    out_scores: [*]f32,
) c_int {
    const handle = h orelse return -1;
    const ctx = handle.ctx orelse return -2;
    const model = handle.model orelse return -3;
    const vocab = c.llama_model_get_vocab(model);

    var prompt_tokens: [8192]c.llama_token = undefined;
    const text_len_i: c_int = @intCast(prompt_len);
    const n_tokens = c.llama_tokenize(vocab, prompt, text_len_i, &prompt_tokens, prompt_tokens.len, false, true);
    if (n_tokens < 0) return -4;
    const n_tok: c_int = n_tokens;

    if (n_cands > 256) return -5;
    var cand_ids: [256]c.llama_token = undefined;
    var i: c_int = 0;
    while (i < n_cands) : (i += 1) {
        const code = cand_codes[@intCast(i)];
        const code_len: c_int = @intCast(std.mem.len(code));
        var ids: [8]c.llama_token = undefined;
        const m = c.llama_tokenize(vocab, code, code_len, &ids, ids.len, false, false);
        if (m != 1) return -6;
        cand_ids[@intCast(i)] = ids[0];
    }

    const mem = c.llama_get_memory(ctx);
    c.llama_memory_clear(mem, true);

    const batch = c.llama_batch_get_one(&prompt_tokens, n_tok);
    if (c.llama_decode(ctx, batch) != 0) return -7;

    const logits = c.llama_get_logits_ith(ctx, n_tok - 1);
    if (logits == null) return -8;
    i = 0;
    while (i < n_cands) : (i += 1) {
        out_scores[@intCast(i)] = logits[@intCast(cand_ids[@intCast(i)])];
    }
    return 0;
}

// --- New method: per-candidate yes/no (LLM2Jev-style) ---
//
// For each candidate, the Go side constructs a full prompt (with chat template)
// ending in "yes" or "no". This function does ONE forward pass and returns the
// logit for the specified yes_token_id at the last position.
//
// fr_score_yesno(handle, prompt, prompt_len, yes_token_id, out_logit) -> int
//   out_logit is a pointer to a single float.

export fn fr_score_yesno(
    h: ?*Handle,
    prompt: [*]const u8,
    prompt_len: usize,
    yes_token_id: c_int,
    out_logit: [*]f32,
) c_int {
    const handle = h orelse return -1;
    const ctx = handle.ctx orelse return -2;
    const vocab = c.llama_model_get_vocab(handle.model orelse return -3);

    // tokenize prompt
    var prompt_tokens: [8192]c.llama_token = undefined;
    const text_len_i: c_int = @intCast(prompt_len);
    const n_tokens = c.llama_tokenize(vocab, prompt, text_len_i, &prompt_tokens, prompt_tokens.len, false, true);
    if (n_tokens < 0) return -4;
    const n_tok: c_int = n_tokens;

    // clear KV (each candidate is independent)
    const mem = c.llama_get_memory(ctx);
    c.llama_memory_clear(mem, true);

    // forward
    const batch = c.llama_batch_get_one(&prompt_tokens, n_tok);
    if (c.llama_decode(ctx, batch) != 0) return -7;

    // get logit for yes_token_id at last position
    const logits = c.llama_get_logits_ith(ctx, n_tok - 1);
    if (logits == null) return -8;
    out_logit[0] = logits[@intCast(yes_token_id)];
    return 0;
}

// --- Batched yes/no with shared-prefix KV reuse ---
//
// All N prompts share a long common token prefix (chat template header + state;
// they differ only in the trailing question sentence). This tokenizes all
// prompts, finds the longest common token prefix (LCP), prefills it ONCE, then
// per candidate decodes only the differing suffix and rewinds the KV via
// llama_memory_seq_rm — cutting forward compute from N full prefills to
// 1 full prefill + N short suffix decodes.
//
// fr_score_yesno_batch(handle, prompts, n_prompts, yes_token_id, out_logits) -> int
//   prompts: array of N null-terminated prompt strings
//   out_logits: array of N floats (yes-token logit per prompt)

const MAX_YN_PROMPTS: usize = 32;
const MAX_YN_TOKENS: usize = 8192;

export fn fr_score_yesno_batch(
    h: ?*Handle,
    prompts: [*]const [*:0]const u8,
    n_prompts: c_int,
    yes_token_id: c_int,
    out_logits: [*]f32,
) c_int {
    const handle = h orelse return -1;
    const ctx = handle.ctx orelse return -2;
    const model = handle.model orelse return -3;
    const vocab = c.llama_model_get_vocab(model);
    const n: usize = @intCast(n_prompts);
    if (n == 0 or n > MAX_YN_PROMPTS) return -5;

    // scratch token arrays: [n][MAX_YN_TOKENS]
    const scratch = std.c.malloc(MAX_YN_PROMPTS * MAX_YN_TOKENS * @sizeOf(c.llama_token)) orelse return -9;
    defer std.c.free(scratch);
    const toks: [*]c.llama_token = @ptrCast(@alignCast(scratch));

    // tokenize each prompt into its slot
    var n_tok: [MAX_YN_PROMPTS]c_int = undefined;
    for (0..n) |i| {
        const p = prompts[i];
        const plen: c_int = @intCast(std.mem.len(p));
        const m = c.llama_tokenize(vocab, p, plen, toks + i * MAX_YN_TOKENS, MAX_YN_TOKENS, false, true);
        if (m < 0) return -4;
        n_tok[i] = m;
    }

    // longest common token prefix across all prompts
    var lcp: usize = @intCast(n_tok[0]);
    for (1..n) |i| {
        const lim = @min(lcp, @as(usize, @intCast(n_tok[i])));
        var k: usize = 0;
        while (k < lim and toks[k] == toks[i * MAX_YN_TOKENS + k]) k += 1;
        lcp = k;
    }
    const lcp_i: c_int = @intCast(lcp);

    const mem = c.llama_get_memory(ctx);
    c.llama_memory_clear(mem, true);

    // prefill the shared prefix once
    if (lcp_i > 0) {
        const batch = c.llama_batch_get_one(toks, lcp_i);
        if (c.llama_decode(ctx, batch) != 0) return -7;
    }

    // per candidate: decode suffix → read yes logit (-1 = last of last decode)
    // → rewind KV to the shared prefix for the next candidate
    var i: usize = 0;
    while (i < n) : (i += 1) {
        const base = toks + i * MAX_YN_TOKENS;
        const nt = n_tok[i];
        if (nt > lcp_i) {
            const batch = c.llama_batch_get_one(base + lcp, nt - lcp_i);
            if (c.llama_decode(ctx, batch) != 0) return -7;
        }
        // nt == lcp: prompt identical to the shared prefix — reuse its last logits
        const logits = c.llama_get_logits_ith(ctx, -1);
        if (logits == null) return -8;
        out_logits[i] = logits.?[@intCast(yes_token_id)];
        if (nt > lcp_i) {
            // rewind: remove positions [lcp, end) so the next candidate starts fresh
            _ = c.llama_memory_seq_rm(mem, 0, lcp_i, -1);
        }
    }
    return 0;
}

// --- Chat template: use llama_chat_apply_template (auto-adapt) ---
//
// fr_apply_template(handle, messages_json, messages_len, out_buf, out_buf_len, add_ass) -> int
//   messages_json: JSON array of [{role, content}, ...]
//   out_buf: caller-allocated buffer for the formatted prompt
//   returns: length of formatted prompt (or -1 on error / buffer too small)
//   add_ass: whether to add assistant prompt (true for scoring, false for completion)

export fn fr_apply_template(
    h: ?*Handle,
    messages_json: [*]const u8,
    messages_len: usize,
    out_buf: [*]u8,
    out_buf_len: usize,
    add_ass: bool,
) c_int {
    _ = h; // handle not needed for chat template

    // Parse messages JSON into (start,len) spans.
    var spans: [64]MsgSpan = undefined;
    const n_msgs = parseMessages(messages_json, messages_len, &spans);
    if (n_msgs <= 0) return -10;

    // llama_chat_message fields are NUL-terminated C strings. Copy each
    // role/content out of the raw JSON into a NUL-terminated arena and
    // unescape JSON escapes (backslash-quote, backslash-n, ...) — pointers
    // into the raw JSON would run past the closing quote (v1.0 bug: the
    // template rendered `user\",\"content\":\"...` and duplicated content).
    const n: usize = @intCast(n_msgs);
    const arena = std.c.malloc(messages_len + (2 * n)) orelse return -11;
    defer std.c.free(arena);
    const dst: [*]u8 = @ptrCast(arena);
    var msgs: [64]c.llama_chat_message = undefined;
    var cur: usize = 0;
    for (0..n) |m| {
        const s = spans[m];
        // role: copy + NUL (roles contain no escapes)
        const role_dst = dst + cur;
        @memcpy(role_dst[0..s.role_len], (messages_json + s.role_start)[0..s.role_len]);
        role_dst[s.role_len] = 0;
        msgs[m].role = @ptrCast(role_dst);
        cur += s.role_len + 1;
        // content: copy with unescaping + NUL
        const content_dst = dst + cur;
        var w: usize = 0;
        var r: usize = 0;
        while (r < s.content_len) {
            const ch = messages_json[s.content_start + r];
            if (ch == '\\' and r + 1 < s.content_len) {
                const nxt = messages_json[s.content_start + r + 1];
                const un: u8 = switch (nxt) {
                    'n' => '\n',
                    't' => '\t',
                    'r' => '\r',
                    else => nxt, // backslash-backslash and backslash-quote (others: keep)
                };
                content_dst[w] = un;
                r += 2;
            } else {
                content_dst[w] = ch;
                r += 1;
            }
            w += 1;
        }
        content_dst[w] = 0;
        msgs[m].content = @ptrCast(content_dst);
        cur += w + 1;
    }

    // Apply chat template (null tmpl = use model's built-in template)
    const written = c.llama_chat_apply_template(
        null, // tmpl: null = model's default
        &msgs,
        @intCast(n_msgs),
        add_ass,
        out_buf,
        @intCast(out_buf_len),
    );
    return written;
}

// Minimal JSON parser for [{role:"...", content:"..."}, ...]
// Message span: (start, len) into the raw JSON buffer.
const MsgSpan = struct {
    role_start: usize = 0,
    role_len: usize = 0,
    content_start: usize = 0,
    content_len: usize = 0,
};

// Returns number of messages parsed, or -1 on error.
fn parseMessages(json: [*]const u8, json_len: usize, out: [*]MsgSpan) c_int {
    var count: c_int = 0;
    var i: usize = 0;
    const end = json_len;

    // Skip whitespace + '['
    while (i < end and (json[i] == ' ' or json[i] == '\n' or json[i] == '\t' or json[i] == '[')) i += 1;

    while (i < end and count < 64) {
        // Find {role:..."content":"..."}
        // Skip to '"role"'
        while (i < end and json[i] != '{') i += 1;
        if (i >= end) break;
        i += 1; // skip {

        var role_start: usize = 0;
        var role_len: usize = 0;
        var content_start: usize = 0;
        var content_len: usize = 0;

        // Simple state machine: find "role" and "content" values
        while (i < end) {
            // skip whitespace and commas
            while (i < end and (json[i] == ' ' or json[i] == ',' or json[i] == '\n' or json[i] == '\t')) i += 1;
            if (i >= end or json[i] == '}') break;

            // expect "key"
            if (json[i] != '"') { i += 1; continue; }
            i += 1; // skip opening quote
            const key_start = i;
            while (i < end and json[i] != '"') i += 1;
            const key_len = i - key_start;
            i += 1; // skip closing quote

            // skip : and whitespace
            while (i < end and (json[i] == ':' or json[i] == ' ' or json[i] == '\n' or json[i] == '\t')) i += 1;

            // expect "value"
            if (i >= end or json[i] != '"') { i += 1; continue; }
            i += 1; // skip opening quote
            const val_start = i;
            while (i < end and json[i] != '"') {
                if (json[i] == '\\') i += 2 else i += 1; // skip escaped chars
            }
            const val_len = i - val_start;
            i += 1; // skip closing quote

            // match key
            if (key_len == 4 and std.mem.eql(u8, json[key_start..key_start+4], "role")) {
                role_start = val_start;
                role_len = val_len;
            } else if (key_len == 7 and std.mem.eql(u8, json[key_start..key_start+7], "content")) {
                content_start = val_start;
                content_len = val_len;
            }
        }

        if (role_len > 0 and content_len > 0 and count < 64) {
            // Store spans; the caller copies them into NUL-terminated buffers
            // (llama_chat_message fields are C strings — pointers into the raw
            // JSON would run past the closing quote; v1.0 bug).
            out[@intCast(count)] = .{
                .role_start = role_start,
                .role_len = role_len,
                .content_start = content_start,
                .content_len = content_len,
            };
            count += 1;
        }

        // skip to next }
        while (i < end and json[i] != '}') i += 1;
        if (i < end) i += 1; // skip }
    }

    return count;
}

// fr_get_token_id: tokenize a single word, return its token id (or -1 if multi-token).
export fn fr_get_token_id(h: ?*Handle, word: [*]const u8, word_len: usize) c_int {
    const handle = h orelse return -1;
    const vocab = c.llama_model_get_vocab(handle.model orelse return -3);
    var ids: [8]c.llama_token = undefined;
    const m = c.llama_tokenize(vocab, word, @intCast(word_len), &ids, ids.len, false, false);
    if (m != 1) return -6;
    return ids[0];
}

export fn fr_free(h: ?*Handle) void {
    const handle = h orelse return;
    if (handle.ctx) |ctx| c.llama_free(ctx);
    if (handle.model) |model| c.llama_free_model(model);
    std.c.free(handle);
}
