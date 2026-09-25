// frwrapper.zig — narrow C ABI wrapper over llama.cpp for Jev system-one scoring.
//
// Hides llama_batch / model_params / context_params structs (handled by zig
// via @cImport) and exposes only scalar+pointer functions for purego to bind:
//   fr_load(path) -> handle
//   fr_score(handle, prompt, prompt_len, cand_codes, n_cands, out_scores) -> int
//   fr_free(handle)
//
// fr_score: tokenize prompt + verify each candidate code is single token +
// llama_batch_get_one + llama_decode + llama_get_logits_ith(last) + extract
// candidate logits into out_scores. Go-side softmaxes.
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
    mp.n_gpu_layers = 0; // CPU (cross-platform default; GPU via separate build)
    const model = c.llama_load_model_from_file(model_path, mp);
    if (model == null) return null;
    var cp = c.llama_context_default_params();
    cp.n_ctx = 4096;
    cp.n_batch = 4096;
    const ctx = c.llama_new_context_with_model(model, cp);
    if (ctx == null) {
        c.llama_free_model(model);
        return null;
    }
    const h: *Handle = @ptrCast(@alignCast(std.c.malloc(@sizeOf(Handle)) orelse return null));
    h.* = .{ .model = model, .ctx = ctx };
    return h;
}

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

    // 1. tokenize prompt
    var prompt_tokens: [8192]c.llama_token = undefined;
    const text_len_i: c_int = @intCast(prompt_len);
    const n_tokens = c.llama_tokenize(vocab, prompt, text_len_i, &prompt_tokens, prompt_tokens.len, false, true);
    if (n_tokens < 0) return -4; // prompt too long for buffer
    const n_tok: c_int = n_tokens;

    // 2. verify each candidate code is a single token, record its id
    if (n_cands > 256) return -5; // Jev limit is 255
    var cand_ids: [256]c.llama_token = undefined;
    var i: c_int = 0;
    while (i < n_cands) : (i += 1) {
        const code = cand_codes[@intCast(i)];
        const code_len: c_int = @intCast(std.mem.len(code));
        var ids: [8]c.llama_token = undefined;
        const m = c.llama_tokenize(vocab, code, code_len, &ids, ids.len, false, false);
        if (m != 1) return -6; // candidate code is not a single token
        cand_ids[@intCast(i)] = ids[0];
    }

    // 3. clear KV memory — each fr_score is an independent prompt; prior
    //    decode state must not accumulate (would fill n_ctx and fail).
    const mem = c.llama_get_memory(ctx);
    c.llama_memory_clear(mem, true);

    // 4. batch (struct handled by zig) + decode
    const batch = c.llama_batch_get_one(&prompt_tokens, n_tok);
    if (c.llama_decode(ctx, batch) != 0) return -7;

    // 4. logits for the last token, extract candidate positions
    const logits = c.llama_get_logits_ith(ctx, n_tok - 1);
    if (logits == null) return -8;
    i = 0;
    while (i < n_cands) : (i += 1) {
        out_scores[@intCast(i)] = logits[@intCast(cand_ids[@intCast(i)])];
    }
    return 0;
}

export fn fr_free(h: ?*Handle) void {
    const handle = h orelse return;
    if (handle.ctx) |ctx| c.llama_free(ctx);
    if (handle.model) |model| c.llama_free_model(model);
    std.c.free(handle);
}
