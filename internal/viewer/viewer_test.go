package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/huaiyuWangh/Hackathon2026/internal/ccrunner"
)

type fakeSource struct {
	ch     chan ccrunner.Chunk
	view   ccrunner.View
	closed int
}

func (f *fakeSource) Subscribe() ccrunner.View {
	return ccrunner.View{
		Cols: 100, Rows: 30, Replay: f.view.Replay, Ch: f.ch,
		Cancel: func() {},
	}
}

func TestViewerPageAndAssets(t *testing.T) {
	mux := http.NewServeMux()
	Mount(mux, &fakeSource{ch: make(chan ccrunner.Chunk)})

	for path, want := range map[string]string{
		"/viewer":           "<!doctype html>",
		"/viewer/xterm.js":  "!function",
		"/viewer/xterm.css": "xterm",
	} {
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != 200 || !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("%s 响应异常: code=%d want含%q", path, resp.Code, want)
		}
	}
}

func TestViewerStream(t *testing.T) {
	src := &fakeSource{ch: make(chan ccrunner.Chunk, 4)}
	src.view.Replay = "replay-frame"
	mux := http.NewServeMux()
	Mount(mux, src)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/viewer/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	// init: 文本 JSON，含尺寸与回放
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var init struct {
		Type   string `json:"type"`
		Cols   int    `json:"cols"`
		Rows   int    `json:"rows"`
		Replay string `json:"replay"`
	}
	if err := json.Unmarshal(msg, &init); err != nil || init.Type != "init" {
		t.Fatalf("init 消息异常: %s", msg)
	}
	if init.Cols != 100 || init.Replay != "replay-frame" {
		t.Fatalf("init 内容异常: %+v", init)
	}

	// 增量: 二进制帧，\f 翻译为清屏序列
	src.ch <- ccrunner.Chunk{Data: []byte("\fhello")}
	mt, delta, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.BinaryMessage || !strings.HasPrefix(string(delta), "\x1b[2J") ||
		!strings.HasSuffix(string(delta), "hello") {
		t.Fatalf("增量帧异常 mt=%d: %q", mt, delta)
	}
}
