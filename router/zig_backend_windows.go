//go:build windows

// Package router: C3 Backend on Windows — syscall.NewLazyDLL (no cgo, no purego).
//
// purego.Dlopen is Unix-only (dlfcn). Windows uses the standard library's
// syscall.NewLazyDLL + LazyProc.Call, which is cgo-free and cross-compiles
// to windows/amd64 from any platform.
package router

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

type zigBackend struct {
	mu      sync.Mutex // llama_context is not thread-safe; serialize every call
	dll     *syscall.DLL
	frLoad  *syscall.Proc
	frScore *syscall.Proc
	frFree  *syscall.Proc
	handle  uintptr
}

func NewZigBackend(libPath, modelPath string) (*zigBackend, error) {
	dll, err := syscall.LoadDLL(libPath)
	if err != nil {
		return nil, err
	}
	b := &zigBackend{dll: dll}
	if b.frLoad, err = dll.FindProc("fr_load"); err != nil {
		return nil, err
	}
	if b.frScore, err = dll.FindProc("fr_score"); err != nil {
		return nil, err
	}
	if b.frFree, err = dll.FindProc("fr_free"); err != nil {
		return nil, err
	}
	cpath, err := syscall.BytePtrFromString(modelPath)
	if err != nil {
		return nil, err
	}
	ret, _, _ := b.frLoad.Call(uintptr(unsafe.Pointer(cpath)))
	if ret == 0 {
		return nil, errors.New("fr_load returned 0 (model load failed — check model path and libllama deps)")
	}
	b.handle = ret
	return b, nil
}

func (b *zigBackend) ChoiceScore(prompt string, codes []string) ([]float64, Usage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cprompt, err := syscall.BytePtrFromString(prompt)
	if err != nil {
		return nil, Usage{}, err
	}
	// Build char** array of null-terminated code strings.
	codePtrs := make([]uintptr, len(codes))
	codeBytes := make([]*byte, len(codes))
	for i, c := range codes {
		cb, err := syscall.BytePtrFromString(c)
		if err != nil {
			return nil, Usage{}, err
		}
		codeBytes[i] = cb
		codePtrs[i] = uintptr(unsafe.Pointer(cb))
	}
	out := make([]float32, len(codes))
	ret, _, _ := b.frScore.Call(
		b.handle,
		uintptr(unsafe.Pointer(cprompt)),
		uintptr(len(prompt)),
		uintptr(unsafe.Pointer(&codePtrs[0])),
		uintptr(len(codes)),
		uintptr(unsafe.Pointer(&out[0])),
	)
	if ret != 0 {
		return nil, Usage{}, frScoreError(int32(ret))
	}
	scores := make([]float64, len(out))
	for i, v := range out {
		scores[i] = float64(v)
	}
	// keep refs alive until after Call
	runtime.KeepAlive(codeBytes)
	runtime.KeepAlive(codePtrs)
	runtime.KeepAlive(out)
	return scores, Usage{InputTokens: len(prompt), OutputTokens: 1}, nil
}

func (b *zigBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.handle != 0 {
		b.frFree.Call(b.handle)
		b.handle = 0
	}
}
