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

    // Parse messages JSON (simple parser — expects [{role, content}, ...])
    // We'll use a minimal JSON parser to extract role/content pairs.
    var msg_buf: [64]c.llama_chat_message = undefined;
    const n_msgs = parseMessages(messages_json, messages_len, &msg_buf);
    if (n_msgs <= 0) return -10;

    // Apply chat template (null tmpl = use model's built-in template)
    const written = c.llama_chat_apply_template(
        null,           // tmpl: null = model's default
        &msg_buf,
        @intCast(n_msgs),
        add_ass,
        out_buf,
        @intCast(out_buf_len),
    );
    return written;
}

// Minimal JSON parser for [{role:"...", content:"..."}, ...]
// Returns number of messages parsed, or -1 on error.
fn parseMessages(json: [*]const u8, json_len: usize, out: [*]c.llama_chat_message) c_int {
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
            // Store pointers (NOTE: these point into the input json buffer,
            // which must remain valid until llama_chat_apply_template returns)
            out[@intCast(count)] = .{
                .role = @ptrCast(&json[role_start]),
                .content = @ptrCast(&json[content_start]),
            };
            count += 1;
        }

        // skip to next }
        while (i < end and json[i] != '}') i += 1;
        if (i < end) i += 1; // skip }
    }

    return count;
}

export fn fr_free(h: ?*Handle) void {
    const handle = h orelse return;
    if (handle.ctx) |ctx| c.llama_free(ctx);
    if (handle.model) |model| c.llama_free_model(model);
    std.c.free(handle);
}
