// Package viewer 提供 CC 现场的网页直播：xterm.js 终端镜像，只读。
// 挂载在独立端口（默认 0.0.0.0:8788）暴露给局域网——手机浏览器直接看；
// 控制页/MCP 留在 127.0.0.1 不出内网。
package viewer

import (
	_ "embed"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/huaiyuWangh/Hackathon2026/internal/ccrunner"
)

//go:embed index.html
var indexHTML []byte

//go:embed xterm.js
var xtermJS []byte

//go:embed xterm.css
var xtermCSS []byte

// Source 现场数据源（ccrunner.Runner 满足）。
type Source interface {
	Subscribe() ccrunner.View
}

// Mount 把查看器挂到 mux：页面 /viewer、静态资源 /viewer/*、WebSocket /viewer/ws。
func Mount(mux *http.ServeMux, src Source) {
	mux.HandleFunc("/viewer", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/viewer/xterm.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write(xtermJS)
	})
	mux.HandleFunc("/viewer/xterm.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(xtermCSS)
	})
	mux.HandleFunc("/viewer/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(w, r, src)
	})
}

var upgrader = websocket.Upgrader{
	// 局域网内任意来源可连（演示场景）；控制面不在此端口
	CheckOrigin: func(*http.Request) bool { return true },
}

// serveWS 单个观看者连接：首条文本消息为 init JSON（尺寸+全量回放），
// 之后增量以二进制帧推送（\f 重绘标记翻译为 xterm 清屏序列）。
func serveWS(w http.ResponseWriter, req *http.Request, src Source) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	v := src.Subscribe()
	defer v.Cancel()

	if err := conn.WriteJSON(map[string]any{
		"type":   "init",
		"cols":   v.Cols,
		"rows":   v.Rows,
		"replay": v.Replay,
	}); err != nil {
		return
	}
	log.Printf("viewer: 新观看者 (cols=%d rows=%d)", v.Cols, v.Rows)

	closed := make(chan struct{})
	go func() { // 只负责感知客户端断开
		defer close(closed)
		conn.SetReadLimit(512)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-closed:
			return
		case c, ok := <-v.Ch:
			if !ok {
				return
			}
			data := strings.ReplaceAll(string(c.Data), "\f", "\x1b[2J\x1b[H")
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte(data)); err != nil {
				return
			}
		}
	}
}
