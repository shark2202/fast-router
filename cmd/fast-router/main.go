// cmd/fast-router: start the smart router gateway.
//
// Loads the Jev scorer (zig+libllama), the seed model registry, and upstream
// config (from env), then serves /v1/chat/completions + /v1/messages.
//
// Env:
//   FR_MODEL     GGUF model path (default: downloaded Qwen2.5-0.5B for POC)
//   FR_LIB       libfrwrapper path (default: zig/libfrwrapper.dylib)
//   FR_LIB_DIR   llama.cpp lib dir for DYLD_LIBRARY_PATH
//   OPENAI_API_KEY / ANTHROPIC_API_KEY / DEEPSEEK_API_KEY
//   FR_ADDR      listen addr (default :8080)
package main

import (
	"log"
	"net/http"
	"os"

	"fast-router/router"
)

func main() {
	modelPath := os.Getenv("FR_MODEL")
	if modelPath == "" {
		log.Fatal("FR_MODEL required (GGUF path, e.g. Qwen2.5-0.5B-Instruct-Q4_K_M.gguf)")
	}
	libPath := os.Getenv("FR_LIB")
	if libPath == "" {
		libPath = "zig/libfrwrapper.dylib"
	}

	backend, err := router.NewZigBackend(libPath, modelPath)
	if err != nil {
		log.Fatalf("backend: %v (DYLD_LIBRARY_PATH=%s set?)", err, os.Getenv("FR_LIB_DIR"))
	}
	defer backend.Close()
	scorer := router.NewScorer(backend, "jev-local")

	upstreams := map[string]router.Upstream{
		"openai":    {Name: "openai", BaseURL: "https://api.openai.com/v1", APIKey: os.Getenv("OPENAI_API_KEY"), Protocol: "openai"},
		"anthropic": {Name: "anthropic", BaseURL: "https://api.anthropic.com", APIKey: os.Getenv("ANTHROPIC_API_KEY"), Protocol: "anthropic"},
		"deepseek":  {Name: "deepseek", BaseURL: "https://api.deepseek.com/v1", APIKey: os.Getenv("DEEPSEEK_API_KEY"), Protocol: "openai"},
	}

	gw := router.NewGateway(scorer, router.SeedRegistry, upstreams)

	addr := os.Getenv("FR_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("fast-router on %s (Jev scorer ready, %d upstreams)", addr, len(upstreams))
	if err := (&http.Server{Addr: addr, Handler: gw}).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
