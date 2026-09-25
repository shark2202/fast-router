// Package router: admin — web UI for config + upstream management.
//
// GET  /admin           → HTML config page (upstream forms + test buttons)
// GET  /api/config      → current config as JSON
// POST /api/config      → update config (persist to file + hot-reload gateway upstreams)
// POST /api/upstream/test → test an upstream's connectivity (HEAD/GET /models)
package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Admin serves the config web UI + REST API, with hot-reload of gateway upstreams.
type Admin struct {
	cfg        *Config
	cfgPath    string
	gateway    *Gateway
	mu         sync.Mutex
}

func NewAdmin(cfg *Config, cfgPath string, gw *Gateway) *Admin {
	return &Admin{cfg: cfg, cfgPath: cfgPath, gateway: gw}
}

// ServeHTTP routes admin paths.
func (a *Admin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/admin", "/admin/":
		a.serveHTML(w, r)
	case "/api/config":
		switch r.Method {
		case http.MethodGet:
			a.getConfig(w, r)
		case http.MethodPost:
			a.postConfig(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	case "/api/upstream/test":
		a.testUpstream(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (a *Admin) getConfig(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a.cfg)
}

func (a *Admin) postConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var newCfg Config
	if err := json.Unmarshal(body, &newCfg); err != nil {
		http.Error(w, "invalid config: "+err.Error(), 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// persist
	if err := newCfg.Save(a.cfgPath); err != nil {
		http.Error(w, "save: "+err.Error(), 500)
		return
	}
	a.cfg = &newCfg
	// hot-reload gateway upstreams
	if a.gateway != nil {
		a.gateway.SetUpstreams(a.cfg.ToUpstreams())
	}
	w.WriteHeader(200)
	fmt.Fprint(w, `{"ok":true}`)
}

// testUpstream: POST /api/upstream/test {name, base_url, api_key, protocol}
// → tries a GET {base_url}/models (OpenAI) or /v1/messages (Anthropic HEAD).
func (a *Admin) testUpstream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		Protocol string `json:"protocol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	url := req.BaseURL + "/models"
	httpReq, _ := http.NewRequest("GET", url, nil)
	if req.Protocol == "anthropic" {
		httpReq.Header.Set("x-api-key", req.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	json.NewEncoder(w).Encode(map[string]any{
		"ok":         resp.StatusCode < 400,
		"status":     resp.StatusCode,
		"upstream":   req.Name,
	})
}

func (a *Admin) serveHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, adminHTML)
}

const adminHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>fast-router admin</title>
<style>
body{font:14px system-ui;max-width:800px;margin:2em auto;padding:0 1em;color:#222}
h1{font-size:1.4em}
.upstream{border:1px solid #ddd;padding:1em;margin:1em 0;border-radius:4px}
label{display:block;margin:0.3em 0;font-size:0.9em}
input{width:100%;padding:0.3em;box-sizing:border-box;font:inherit}
button{padding:0.4em 1em;margin:0.2em 0;cursor:pointer}
.ok{color:#080}.err{color:#c00}
</style></head><body>
<h1>fast-router config</h1>
<div id="app">loading…</div>
<script>
async function load(){
  const r = await fetch('/api/config');
  const cfg = await r.json();
  const app = document.getElementById('app');
  let h = '<label>listen <input id="listen" value="'+cfg.listen+'"></label>';
  h += '<label>model path <input id="modelpath" value="'+(cfg.model.path||'')+'"></label>';
  h += '<label>lib <input id="lib" value="'+(cfg.model.lib||'')+'"></label>';
  h += '<label>lib_dir <input id="libdir" value="'+(cfg.model.lib_dir||'')+'"></label>';
  h += '<h2>upstreams</h2><div id="ups"></div>';
  h += '<button onclick="addUp()">+ add upstream</button> ';
  h += '<button onclick="save()">save</button>';
  app.innerHTML = h;
  renderUps(cfg.upstreams||{});
}
function renderUps(ups){
  const d = document.getElementById('ups');
  d.innerHTML='';
  for(const[name,u]of Object.entries(ups)){
    d.innerHTML+='<div class="upstream"><b>'+name+'</b>'+
      '<label>base_url <input class="bu" data-n="'+name+'" value="'+u.base_url+'"></label>'+
      '<label>api_key <input class="ak" data-n="'+name+'" value="'+u.api_key+'" type="password"></label>'+
      '<label>protocol <input class="pr" data-n="'+name+'" value="'+u.protocol+'"></label>'+
      '<button onclick="testUp(\''+name+'\')">test</button> '+
      '<button onclick="delUp(\''+name+'\')">delete</button> '+
      '<span class="res" id="res-'+name+'"></span></div>';
  }
}
function addUp(){
  const n=prompt('upstream name (e.g. openai/anthropic/deepseek)');
  if(!n)return;
  renderUps(Object.assign(currentUps(),{[n]:{base_url:'',api_key:'',protocol:'openai'}}));
}
function delUp(n){
  const u=currentUps();delete u[n];renderUps(u);
}
function currentUps(){
  const ups={};
  document.querySelectorAll('.upstream').forEach(d=>{
    const n=d.querySelector('.bu').dataset.n;
    ups[n]={base_url:d.querySelector('.bu').value,api_key:d.querySelector('.ak').value,protocol:d.querySelector('.pr').value};
  });
  return ups;
}
async function testUp(n){
  const u=currentUps()[n];
  const el=document.getElementById('res-'+n);
  el.textContent='testing…';
  const r=await fetch('/api/upstream/test',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(Object.assign({name:n},u))});
  const j=await r.json();
  el.innerHTML=j.ok?'<span class="ok">ok ('+j.status+')</span>':'<span class="err">fail ('+j.status+')</span>';
}
async function save(){
  const cfg={listen:document.getElementById('listen').value,
    model:{path:document.getElementById('modelpath').value,lib:document.getElementById('lib').value,lib_dir:document.getElementById('libdir').value},
    upstreams:currentUps()};
  const r=await fetch('/api/config',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(cfg)});
  alert(r.ok?'saved':'save failed');
}
load();
</script>
</body></html>`
