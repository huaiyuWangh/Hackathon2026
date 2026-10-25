package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// SSEServer 是 MCP 的 HTTP+SSE 传输（灵矽云端 MCP 接入形态）：
// GET /mcp 建立 SSE 长连接（首个 endpoint 事件告知回传地址，带 sessionId），
// POST /mcp/message 接收平台的 JSON-RPC 请求，响应写回对应会话的 SSE 流。
// 平台调用时自动携带 X-Linx-Device-Id / X-Linx-Agent-Id / X-Linx-Session-Id。
type SSEServer struct {
	srv      *Server
	mux      *http.ServeMux
	mu       sync.Mutex
	sessions map[string]chan []byte
}

// NewSSEServer 创建 SSE 传输服务（路由表仅此一份，供控制页等挂载）。
func NewSSEServer(p ToolProvider) *SSEServer {
	s := &SSEServer{srv: NewServer(p), sessions: make(map[string]chan []byte)}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/mcp", s.handleStream)
	s.mux.HandleFunc("/mcp/message", s.handleMessage)
	return s
}

// Handler 返回 HTTP 处理器。
func (s *SSEServer) Handler() http.Handler {
	return s.mux
}

// Mux 返回路由表（供控制页等额外路由挂载到同一端口）。
func (s *SSEServer) Mux() *http.ServeMux {
	return s.mux
}

// ListenAndServe 阻塞监听（配合 cloudflared 等隧道暴露给平台）。
func (s *SSEServer) ListenAndServe(addr string) error {
	log.Printf("mcp-sse 监听 http://%s/mcp", addr)
	return http.ListenAndServe(addr, s.Handler())
}

func (s *SSEServer) handleStream(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	id := randomHex(16)
	ch := make(chan []byte, 32)
	s.mu.Lock()
	s.sessions[id] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.sessions, id)
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// 标准 MCP SSE 握手：endpoint 事件告知请求回传地址
	fmt.Fprintf(w, "event: endpoint\ndata: /mcp/message?sessionId=%s\n\n", id)
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		case msg := <-ch:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func (s *SSEServer) handleMessage(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var rpc Request
	if err := json.NewDecoder(req.Body).Decode(&rpc); err != nil || rpc.Method == "" {
		http.Error(w, "invalid json-rpc", http.StatusBadRequest)
		return
	}
	if d := req.Header.Get("X-Linx-Agent-Id"); d != "" {
		log.Printf("mcp-sse: %s (agent %s…)", rpc.Method, d[:min(8, len(d))])
	} else {
		log.Printf("mcp-sse: %s", rpc.Method)
	}
	resp := s.srv.Handle(req.Context(), &rpc)
	if len(rpc.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	out, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "marshal failed", http.StatusInternalServerError)
		return
	}
	ch := s.session(req.URL.Query().Get("sessionId"))
	if ch == nil {
		// 没有关联 SSE 流时退化为同步 HTTP 响应（兼容直连调试）
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
		return
	}
	select {
	case ch <- out:
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "session stream stalled", http.StatusBadGateway)
	}
}

func (s *SSEServer) session(id string) chan []byte {
	if id == "" {
		// 未带 sessionId：若只有一个活跃流则使用它
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.sessions) == 1 {
			for _, ch := range s.sessions {
				return ch
			}
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
