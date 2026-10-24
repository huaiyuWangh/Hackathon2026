// Package webui 提供设备控制页：中间一个大按钮开关麦克风。
package webui

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/huaiyuWangh/Hackathon2026/internal/brand"
)

//go:embed index.html
var indexHTML []byte

// Controller 控制页可操作的设备能力。
type Controller interface {
	// SetMic 开/关麦克风，返回切换后的状态（未启用音频时返回错误）。
	SetMic(on bool) (bool, error)
	// MicOn 当前麦克风状态。
	MicOn() bool
}

// Mount 把控制页与 API 挂到 mux（页面在 /，API 在 /api/*）。
func Mount(mux *http.ServeMux, c Controller) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("/icon.svg", brand.ServeIcon)
	// POST /api/mic 切换（body 可选 {"on":true} 指定目标状态），返回新状态
	mux.HandleFunc("/api/mic", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			On *bool `json:"on"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		target := !c.MicOn()
		if in.On != nil {
			target = *in.On
		}
		on, err := c.SetMic(target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"micOn": on})
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"micOn": c.MicOn()})
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
