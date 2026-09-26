// cmd/fast-router: start the smart router gateway with config file.
//
// Loads config from --config (default ./fast-router.json or FR_CONFIG env),
// initializes the Jev scorer (zig+libllama), the model registry, and upstreams.
// Serves /v1/* (gateway) + /admin + /api/* (admin web UI for config).
//
// Usage:
//   DYLD_LIBRARY_PATH=/tmp/llama-bins/llama-b11175 \
//   ./fast-router --config ./fast-router.json
//
// On first run (no config file), writes a default template (no API keys —
// fill them via http://localhost:8080/admin).
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"fast-router/router"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "", "config file path (default ./fast-router.json or FR_CONFIG env)")
	flag.Parse()

	if cfgPath == "" {
		cfgPath = os.Getenv("FR_CONFIG")
	}
	if cfgPath == "" {
		cfgPath = "fast-router.json"
	}

	// Load config (creates default template if file missing).
	cfg, err := router.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("load config %s: %v", cfgPath, err)
	}
	// If the file didn't exist, save the default template so the user can edit it.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := cfg.Save(cfgPath); err != nil {
			log.Printf("warn: could not write default config template: %v", err)
		} else {
			log.Printf("wrote default config template to %s — edit it or use the admin UI", cfgPath)
		}
	}

	registry := cfg.Registry
	if len(registry) == 0 {
		registry = router.SeedRegistry
	}

	// Initialize the Jev scorer (zig+libllama) if model path is configured.
	var gw *router.Gateway
	if cfg.Model.Path != "" {
		backend, err := router.NewZigBackend(cfg.Model.Lib, cfg.Model.Path)
		if err != nil {
			log.Fatalf("backend: %v (DYLD_LIBRARY_PATH / LD_LIBRARY_PATH set to %s?)", err, cfg.Model.LibDir)
		}
		defer backend.Close()
		scorer := router.NewScorer(backend, "jev-local")
		engine := router.NewNativeSystemOneEngine(scorer)
		gw = router.NewGateway(engine, registry, cfg.ToUpstreams())
		log.Printf("Jev scorer ready (model=%s)", cfg.Model.Path)
	} else {
		// No model configured — gateway runs in hint-only mode (model-name strong hint bypasses Jev).
		gw = router.NewGateway(nil, registry, cfg.ToUpstreams())
		log.Printf("no model configured — running in hint-only mode (set model.path via admin UI to enable Jev)")
	}

	admin := router.NewAdmin(cfg, cfgPath, gw)

	// Mux: /v1/* → gateway, /admin + /api/* → admin, else → gateway (fallback).
	mux := http.NewServeMux()
	mux.Handle("/v1/", gw)
	mux.Handle("/admin", admin)
	mux.Handle("/admin/", admin)
	mux.Handle("/api/", admin)
	mux.Handle("/", gw) // /v1/chat/completions etc. also via gateway

	addr := cfg.Listen
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("fast-router on %s (config: %s, %d upstreams) — admin UI at http://localhost%s/admin",
		addr, cfgPath, len(cfg.Upstreams), addr)
	if err := (&http.Server{Addr: addr, Handler: mux}).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
