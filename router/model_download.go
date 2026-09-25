// Package router: model download — modelscope GGUF download for first-run.
//
// On first run (no model.path in config), the admin UI offers a model picker.
// This handler downloads a GGUF model from modelscope and returns the local
// path, which the user saves into config.model.path via /api/config.
package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// AvailableModels: models the admin UI offers for download (modelscope GGUF).
// Sizes are approximate (q4_k_m); costs are disk + RAM at load.
var AvailableModels = []ModelOption{
	{ID: "Qwen/Qwen2.5-0.5B-Instruct-GGUF", Name: "Qwen2.5-0.5B (fastest, ~0.5GB, low accuracy)", Size: "0.5GB", RAM: "1GB", File: "qwen2.5-0.5b-instruct-q4_k_m.gguf"},
	{ID: "Qwen/Qwen2.5-1.5B-Instruct-GGUF", Name: "Qwen2.5-1.5B (~3GB, better accuracy)", Size: "3GB", RAM: "4GB", File: "qwen2.5-1.5b-instruct-q4_k_m.gguf"},
	{ID: "Qwen/Qwen2.5-3B-Instruct-GGUF", Name: "Qwen2.5-3B (~2GB, good balance)", Size: "2GB", RAM: "4GB", File: "qwen2.5-3b-instruct-q4_k_m.gguf"},
	{ID: "Qwen/Qwen3-8B-GGUF", Name: "Qwen3-8B (~5GB, high accuracy, slow on CPU)", Size: "5GB", RAM: "6GB", File: "Qwen3-8B-Q4_K_M.gguf"},
	{ID: "Qwen/Qwen2.5-14B-Instruct-GGUF", Name: "Qwen2.5-14B (~9GB, highest, needs GPU)", Size: "9GB", RAM: "10GB", File: "qwen2.5-14b-instruct-q4_k_m.gguf"},
}

type ModelOption struct {
	ID   string `json:"id"`    // modelscope repo id
	Name string `json:"name"`  // display name
	Size string `json:"size"`   // disk size
	RAM  string `json:"ram"`    // runtime RAM
	File string `json:"file"`   // gguf filename
}

// ModelService: download models via modelscope (through a Python helper) or
// direct HTTP. Since modelscope SDK is Python-only, we shell out to it.
// Falls back to direct modelscope.cn download URL if SDK unavailable.
type ModelService struct {
	cacheDir string // default ~/.cache/modelscope
}

func NewModelService() *ModelService {
	dir, _ := os.UserHomeDir()
	return &ModelService{cacheDir: filepath.Join(dir, ".cache", "modelscope")}
}

// ModelsHandler: GET /api/models (list available) ; POST /api/models/download (start).
// Download is async — status via GET /api/models/download/status.
func (a *Admin) modelsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/models":
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(AvailableModels)
			return
		}
	case "/api/models/download":
		if r.Method == http.MethodPost {
			a.startDownload(w, r)
			return
		}
	case "/api/models/download/status":
		if r.Method == http.MethodGet {
			a.downloadStatus(w, r)
			return
		}
	}
	http.Error(w, "not found", 404)
}

func (a *Admin) startDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// validate model id
	valid := false
	for _, m := range AvailableModels {
		if m.ID == req.ModelID {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "unknown model", 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.downloadInProgress {
		http.Error(w, "download already in progress", 409)
		return
	}
	a.downloadInProgress = true
	a.downloadModel = req.ModelID
	a.downloadErr = ""
	go func() {
		path, err := downloadModel(req.ModelID)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.downloadInProgress = false
		if err != nil {
			a.downloadErr = err.Error()
			return
		}
		a.downloadPath = path
		// auto-set config.model.path
		a.cfg.Model.Path = path
		_ = a.cfg.Save(a.cfgPath)
	}()
	w.WriteHeader(202)
	fmt.Fprint(w, `{"ok":true,"status":"started"}`)
}

func (a *Admin) downloadStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"in_progress": a.downloadInProgress,
		"model":        a.downloadModel,
		"path":         a.downloadPath,
		"error":        a.downloadErr,
	})
}

// downloadModel shells out to modelscope python SDK.
func downloadModel(modelID string) (string, error) {
	// Try modelscope SDK first (more reliable, handles resumability).
	// We use a small python snippet.
	return downloadViaModelscope(modelID)
}

// ModelsHandler is wired to the admin via a field; here we add the fields.
// (Defined in admin.go's Admin struct — add downloadInProgress etc.)

// stub kept for io import
var _ = io.Discard
