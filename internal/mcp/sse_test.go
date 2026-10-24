package mcp

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// openStream 建立 SSE 流并消费首个 endpoint 事件，返回会话 ID、流读取器与
// context 取消函数（须由调用方持有到 resp.Body.Close() 之后，否则流会被提前拆除）。
func openStream(t *testing.T, client *http.Client, base string) (sid string, br *bufio.Reader, resp *http.Response, cancel context.CancelFunc) {
	t.Helper()
	ctx, cn := context.WithTimeout(context.Background(), 10*time.Second)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/mcp", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE 连接失败: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type 应为 text/event-stream: %q", ct)
	}
	br = bufio.NewReader(resp.Body)
	event, data := readEvent(t, br)
	if event != "endpoint" || !strings.HasPrefix(data, "/mcp/message?sessionId=") {
		t.Fatalf("首个事件应为 endpoint 握手: event=%q data=%q", event, data)
	}
	sid = strings.TrimPrefix(data, "/mcp/message?sessionId=")
	if len(sid) != 32 {
		t.Fatalf("sessionId 应为 32 位 hex: %q", sid)
	}
	return sid, br, resp, cn
}

// readEvent 读一个完整 SSE 事件（到空行为止），跳过 :ping 注释行。
func readEvent(t *testing.T, br *bufio.Reader) (event, data string) {
	t.Helper()
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("读 SSE 事件失败: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		if line == "" {
			if event == "" && data == "" {
				continue // 跳过流开头的空行
			}
			return event, data
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func post(t *testing.T, client *http.Client, url, body string) *http.Response {
	t.Helper()
	resp, err := client.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	resp.Body.Close()
	return resp
}

func TestSSEStreamCarriesResponses(t *testing.T) {
	srv := httptest.NewServer(NewSSEServer(fakeProvider{}).Handler())
	defer srv.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	sid, br, resp, cancel := openStream(t, client, srv.URL)
	defer cancel()
	defer resp.Body.Close()

	// 带会话的请求 → 响应走 SSE 流（HTTP 只回 202）
	r := post(t, client, srv.URL+"/mcp/message?sessionId="+sid,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("带会话请求应 202: %d", r.StatusCode)
	}
	event, data := readEvent(t, br)
	if event != "message" || !strings.Contains(data, "self.test.ping") {
		t.Fatalf("SSE 应回带工具列表: event=%q data=%s", event, data)
	}
}

func TestSSEMessageValidationAndFallback(t *testing.T) {
	s := NewSSEServer(fakeProvider{})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	if r := post(t, client, srv.URL+"/mcp/message", `not-json`); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400: %d", r.StatusCode)
	}
	if r := post(t, client, srv.URL+"/mcp/message", `{"method":""}`); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺 method 应 400: %d", r.StatusCode)
	}

	// 无活跃流 → 同步 HTTP 响应（直连调试兼容路径）
	resp, err := client.Post(srv.URL+"/mcp/message", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(buf.String(), "self.test.ping") {
		t.Fatalf("无会话应同步回 JSON: %d %s", resp.StatusCode, buf.String())
	}

	// 通知（无 id）不产生响应体，直接 202
	r := post(t, client, srv.URL+"/mcp/message",
		`{"jsonrpc":"2.0","method":"tools/list"}`)
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("通知应 202: %d", r.StatusCode)
	}

	// 方法守卫
	if r, _ := http.Get(srv.URL + "/mcp/message"); r.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mcp/message 应 405: %d", r.StatusCode)
	}
	resp2, _ := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader("{}"))
	if resp2.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /mcp 应 405: %d", resp2.StatusCode)
	}
}

func TestSSESessionPickRules(t *testing.T) {
	s := NewSSEServer(fakeProvider{})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	// 恰有一条活跃流：不带 sessionId 也能路由到它
	sid, br, resp, cancel := openStream(t, client, srv.URL)
	defer cancel()
	defer resp.Body.Close()
	_ = sid
	r := post(t, client, srv.URL+"/mcp/message", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("单流时无 sessionId 应命中: %d", r.StatusCode)
	}
	if _, data := readEvent(t, br); !strings.Contains(data, "self.test.ping") {
		t.Fatalf("响应应到达唯一活跃流: %s", data)
	}

	// 两条活跃流：无 sessionId 无法唯一定位 → 同步回退
	_, _, resp2, cancel2 := openStream(t, client, srv.URL)
	defer cancel2()
	defer resp2.Body.Close()
	resp3, err := client.Post(srv.URL+"/mcp/message", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("多流且无 sessionId 应同步回退: %d", resp3.StatusCode)
	}
}

func TestSSESlowStreamStalls(t *testing.T) {
	s := NewSSEServer(fakeProvider{})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// 模拟 SSE 消费端彻底卡死：注入一个已满且无人排空的会话通道，
	// POST 应落入 default 分支报 stalled 而不是阻塞等待。
	ch := make(chan []byte, 1)
	ch <- []byte("stuck")
	s.mu.Lock()
	s.sessions["deadbeef"] = ch
	s.mu.Unlock()

	r := post(t, srv.Client(), srv.URL+"/mcp/message?sessionId=deadbeef",
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if r.StatusCode != http.StatusBadGateway {
		t.Fatalf("通道塞满后应 502: %d", r.StatusCode)
	}
}

func TestRandomHex(t *testing.T) {
	if got := randomHex(8); len(got) != 16 {
		t.Fatalf("randomHex(8) 应 16 字符: %q", got)
	}
	// 存入变量再比较，避免 staticcheck SA4000 误判为恒等表达式
	a, b := randomHex(8), randomHex(8)
	if a == b {
		t.Fatal("两次随机值不应相同")
	}
}
