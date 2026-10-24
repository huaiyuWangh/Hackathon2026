package linx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestProxyClientEndToEnd 用 fake 代理服务端走完整 ProxyClient 生命周期：
// 取 token → WS 连接 → initialize/tools/list 应答 → 通知与乱码被忽略 → 断开重连。
func TestProxyClientEndToEnd(t *testing.T) {
	var conns atomic.Int32
	msgOK := make(chan string, 8)

	up := websocket.Upgrader{}
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conns.Add(1) > 2 {
			t.Errorf("重连次数超出预期: %d", conns.Load())
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		read := func() string {
			_, m, err := conn.ReadMessage()
			if err != nil {
				return ""
			}
			return string(m)
		}

		// initialize → 应答带 protocolVersion
		conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
		if m := read(); !strings.Contains(m, "protocolVersion") {
			t.Errorf("initialize 应答异常: %s", m)
		}
		// 二进制帧与乱码文本应被忽略（不回包、不断连）
		conn.WriteMessage(websocket.BinaryMessage, []byte{0x00, 0x01})
		conn.WriteMessage(websocket.TextMessage, []byte(`not-json`))
		// 通知（无 id）不应有回包；随后的 tools/list 仍应正常应答
		conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"jsonrpc":"2.0","method":"tools/list"}`))
		conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
		if m := read(); !strings.Contains(m, "self.test.ping") {
			t.Errorf("tools/list 应答异常: %s", m)
		} else {
			msgOK <- m
		}
		// 服务端断开 → 客户端应重连（第二次走同样流程）
	}))
	defer wsSrv.Close()

	// token 端点：校验 Bearer，返回指向 fake 代理的 wss 地址
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mcpproxy/token" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"code":0,"data":{"wss_endpoint":"` +
			"ws" + strings.TrimPrefix(wsSrv.URL, "http") + `"}}`))
	}))
	defer tokenSrv.Close()

	pc := NewProxyClient(tokenSrv.URL, "k", fakeProvider{})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- pc.Run(ctx) }()

	// 等两轮完整握手流程后取消，Run 应优雅退出
	for i := 0; i < 2; i++ {
		select {
		case <-msgOK:
		case <-time.After(6 * time.Second):
			t.Fatalf("第 %d 轮握手超时", i+1)
		}
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run 应在 ctx 取消后优雅退出: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Run 未退出")
	}
}

func TestProxyClientRequiresAPIKey(t *testing.T) {
	pc := NewProxyClient("http://127.0.0.1:1", "", fakeProvider{})
	if err := pc.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "BRIDGE_API_KEY") {
		t.Fatalf("缺凭证应报错: %v", err)
	}
}

func TestProxyTokenFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":401}`)) // code != 0
	}))
	defer srv.Close()
	pc := NewProxyClient(srv.URL, "k", fakeProvider{})
	if _, err := pc.token(context.Background()); err == nil {
		t.Fatal("code!=0 应报错")
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{bad json`))
	}))
	defer srv2.Close()
	pc2 := NewProxyClient(srv2.URL, "k", fakeProvider{})
	if _, err := pc2.token(context.Background()); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
	// 连接不可达
	pc3 := NewProxyClient("http://127.0.0.1:1", "k", fakeProvider{})
	if _, err := pc3.token(context.Background()); err == nil {
		t.Fatal("不可达应报错")
	}
}
