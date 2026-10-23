package linx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/huaiyuWangh/Hackathon2026/internal/config"
)

// TestClientEndToEnd 用 fake WebSocket 服务端走完整客户端生命周期：
// header 鉴权 → hello 握手 → tools/list 应答 → stt 忽略 → 服务端断开 → 重连。
func TestClientEndToEnd(t *testing.T) {
	events := make(chan [2]string, 16)
	conns := make(chan int, 4)
	release := make(chan struct{})
	defer close(release)

	up := websocket.Upgrader{}
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		cn := n
		conns <- cn

		if r.Header.Get("Authorization") != "Bearer k" ||
			r.Header.Get("Device-Id") != "aa:bb:cc" ||
			r.Header.Get("Client-Id") != "c1" ||
			r.Header.Get("Protocol-Version") != "1" {
			events <- [2]string{"bad-header", ""}
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		_, h, err := conn.ReadMessage()
		if err != nil {
			events <- [2]string{"read-hello-err", err.Error()}
			return
		}
		events <- [2]string{"hello", string(h)}

		_ = conn.WriteJSON(map[string]any{"type": "hello", "session_id": "s1"})
		if cn == 1 {
			send := func(s string) { _ = conn.WriteMessage(websocket.TextMessage, []byte(s)) }
			send(`{"type":"mcp","payload":{"jsonrpc":"2.0","id":1,"method":"tools/list"}}`)
			_, resp, err := conn.ReadMessage()
			if err != nil {
				events <- [2]string{"read-resp-err", err.Error()}
				return
			}
			events <- [2]string{"resp1", string(resp)}

			// stt 应被忽略且不影响连接：随后再发 tools/list 应仍有应答
			send(`{"type":"stt","payload":{"text":"你好"}}`)
			send(`{"type":"mcp","payload":{"jsonrpc":"2.0","id":2,"method":"tools/list"}}`)
			_, resp2, err := conn.ReadMessage()
			if err != nil {
				events <- [2]string{"read-resp2-err", err.Error()}
				return
			}
			events <- [2]string{"resp2", string(resp2)}
			return // 服务端断开 → 客户端应重连
		}
		select { // 第二次连接挂住等测试结束
		case <-release:
		case <-time.After(3 * time.Second):
		}
	}))
	defer srv.Close()

	cli := New(&config.Config{
		LinxWSURL: "ws" + strings.TrimPrefix(srv.URL, "http"),
		DeviceID:  "aa:bb:cc",
		ClientID:  "c1",
		APIKey:    "k",
	}, fakeProvider{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- cli.Run(ctx) }()

	var sawHello, sawResp1, sawResp2 bool
	connsCount := 0
	for !(sawHello && sawResp1 && sawResp2 && connsCount >= 2) {
		select {
		case c := <-conns:
			connsCount = c
		case e := <-events:
			switch e[0] {
			case "hello":
				sawHello = true
				for _, want := range []string{`"version":1`, `"mcp":true`, `"transport":"websocket"`, `"audio_params":{}`} {
					if !strings.Contains(e[1], want) {
						t.Errorf("hello 缺少 %s: %s", want, e[1])
					}
				}
			case "resp1":
				sawResp1 = true
				if !strings.Contains(e[1], "self.test.ping") {
					t.Errorf("resp1 应含工具描述: %s", e[1])
				}
			case "resp2":
				sawResp2 = true
				if !strings.Contains(e[1], `"id":2`) {
					t.Errorf("resp2 应回带 id 2: %s", e[1])
				}
			default:
				t.Fatalf("异常事件: %v", e)
			}
		case <-time.After(6 * time.Second):
			t.Fatalf("超时: hello=%v resp1=%v resp2=%v conns=%d", sawHello, sawResp1, sawResp2, connsCount)
		}
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run 应在 ctx 取消后优雅退出: %v", err)
	}
}

func TestRunRequiresAPIKey(t *testing.T) {
	cli := New(&config.Config{LinxWSURL: "ws://127.0.0.1:1", DeviceID: "d", ClientID: "c"}, fakeProvider{})
	if err := cli.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "BRIDGE_API_KEY") {
		t.Fatalf("缺凭证应报错: %v", err)
	}
}
