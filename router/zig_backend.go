//go:build !windows

// Package router: C3 Backend — zig wrapper + libllama, via purego (no cgo).
//
// dlopen libfrwrapper.{so,dylib,dll} (built from zig/frwrapper.zig), bind the
// narrow C ABI (fr_load/fr_score/fr_free), and implement router.Backend.
// This is the local, in-process, system-one inference backend. Struct layout
// (llama_batch/model_params/context_params) is handled by zig via @cImport,
// so Go-side purego only binds scalars+pointers — no struct ABI risk.
package router

import (
	"errors"
	"unsafe"

	"github.com/ebitengine/purego"
)

// zigBackend implements Backend by dlopen-ing libfrwrapper and calling
// fr_load/fr_score/fr_free. The zig wrapper hides llama.cpp structs.
type zigBackend struct {
	frLoad func(path *byte) unsafe.Pointer
	frScore func(h unsafe.Pointer, prompt *byte, promptLen uintptr, codes **byte, nCands int32, out *float32) int32
	frFree  func(h unsafe.Pointer)
	handle unsafe.Pointer
}

// NewZigBackend opens libfrwrapper and loads a GGUF model.
// libPath: path to libfrwrapper.{so,dylib,dll}
// modelPath: path to a .gguf model file
// Both resolve relative to the working dir; for distribution, the zip lays
// out fast-router + lib/libfrwrapper.* + lib/libllama.* + models/*.gguf.
func NewZigBackend(libPath, modelPath string) (*zigBackend, error) {
	lib, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	b := &zigBackend{}
	purego.RegisterLibFunc(&b.frLoad, lib, "fr_load")
	purego.RegisterLibFunc(&b.frScore, lib, "fr_score")
	purego.RegisterLibFunc(&b.frFree, lib, "fr_free")
	// fr_load takes a null-terminated C string.
	cpath, err := cString(modelPath)
	if err != nil {
		return nil, err
	}
	defer cStringFree(cpath)
	b.handle = b.frLoad(cpath)
	if b.handle == nil {
		return nil, errors.New("fr_load returned nil (model load failed — check model path and libllama deps)")
	}
	return b, nil
}

// ChoiceScore implements Backend: tokenize+verify candidates+decode+logits.
// The zig wrapper does the heavy lifting; we just marshal strings and softmax.
func (b *zigBackend) ChoiceScore(prompt string, codes []string) ([]float64, Usage, error) {
	// Build C prompt (null-terminated).
	cprompt, err := cString(prompt)
	if err != nil {
		return nil, Usage{}, err
	}
	defer cStringFree(cprompt)
	// Build C array of null-terminated code strings: **byte (char**).
	codePtrs, codeFrees := buildCStringArray(codes)
	defer freeCStringArray(codePtrs, codeFrees)
	out := make([]float32, len(codes))
	ret := b.frScore(
		b.handle,
		cprompt,
		uintptr(len(prompt)),
		codePtrs,
		int32(len(codes)),
		&out[0],
	)
	if ret != 0 {
		return nil, Usage{}, frScoreError(ret)
	}
	scores := make([]float64, len(out))
	for i, v := range out {
		scores[i] = float64(v)
	}
	return scores, Usage{InputTokens: len(prompt), OutputTokens: 1}, nil
}

// Close releases the model/context.
func (b *zigBackend) Close() {
	if b.handle != nil {
		b.frFree(b.handle)
		b.handle = nil
	}
}

// --- cgo-free C string helpers ---

// cString allocates a null-terminated byte slice (caller frees via cStringFree).
func cString(s string) (*byte, error) {
	b := make([]byte, len(s)+1)
	copy(b, s)
	// return pointer to first byte; slice kept alive via the returned ptr's
	// underlying array — store header for free.
	return &b[0], nil
}

func cStringFree(p *byte) {
	_ = p // slice GC'd once no refs; nothing to do
}

// buildCStringArray builds a **byte (char**) from Go strings. Returns the
// pointer to the array and a list of backing slices to free.
func buildCStringArray(strs []string) (**byte, [][]byte) {
	if len(strs) == 0 {
		return nil, nil
	}
	backing := make([][]byte, len(strs))
	ptrs := make([]*byte, len(strs))
	for i, s := range strs {
		b := make([]byte, len(s)+1)
		copy(b, s)
		backing[i] = b
		ptrs[i] = &b[0]
	}
	// ptrs is []*byte; we need **byte. Use unsafe to get address of ptrs[0].
	arrPtr := &ptrs[0]
	return (**byte)(unsafe.Pointer(arrPtr)), backing
}

func freeCStringArray(_ **byte, _ [][]byte) {
	// slices GC'd once ptrs/backing unreferenced
}


