package webui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeCtl struct {
	on  bool
	err error
}

func (f *fakeCtl) SetMic(on bool) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.on = on
	return f.on, nil
}

func (f *fakeCtl) MicOn() bool { return f.on }

func newCtlServer(t *testing.T, ctl *fakeCtl) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	Mount(mux, ctl)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func getBody(t *testing.T, url string) (string, *http.Response) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b), resp
}

func TestControlPageServed(t *testing.T) {
	srv := newCtlServer(t, &fakeCtl{})
	body, resp := getBody(t, srv.URL+"/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("首页应 200: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("首页 Content-Type 异常: %q", ct)
	}
	if !strings.Contains(body, "VoiceCopilot") {
		t.Fatal("首页应含品牌名")
	}
	if _, resp := getBody(t, srv.URL+"/nope"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("未知路径应 404: %d", resp.StatusCode)
	}
	// 图标
	body, resp = getBody(t, srv.URL+"/icon.svg")
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "svg") {
		t.Fatalf("图标应 200 svg: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, "<svg") {
		t.Fatal("图标内容应为 SVG")
	}
}

func TestMicAPIToggleAndErrors(t *testing.T) {
	ctl := &fakeCtl{}
	srv := newCtlServer(t, ctl)

	// 无 body：切换语义（false → true）
	_, resp := getBody(t, srv.URL+"/api/mic") // GET → 405
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/mic 应 405: %d", resp.StatusCode)
	}
	r, err := http.Post(srv.URL+"/api/mic", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != http.StatusOK || !strings.Contains(string(b), `"micOn":true`) {
		t.Fatalf("空 body 应切换开: %d %s", r.StatusCode, b)
	}

	// 指定目标状态
	r, _ = http.Post(srv.URL+"/api/mic", "application/json", strings.NewReader(`{"on":false}`))
	b, _ = io.ReadAll(r.Body)
	r.Body.Close()
	if !strings.Contains(string(b), `"micOn":false`) || ctl.MicOn() {
		t.Fatalf("指定 on=false 应关闭: %s", b)
	}

	// 坏 JSON 等价于切换语义（body 解码失败被忽略）
	r, _ = http.Post(srv.URL+"/api/mic", "application/json", strings.NewReader(`bad`))
	b, _ = io.ReadAll(r.Body)
	r.Body.Close()
	if !strings.Contains(string(b), `"micOn":true`) {
		t.Fatalf("坏 JSON 应走切换: %s", b)
	}

	// 设备层错误 → 409
	ctlErr := &fakeCtl{err: errors.New("音频未启用")}
	srv2 := newCtlServer(t, ctlErr)
	r, _ = http.Post(srv2.URL+"/api/mic", "application/json", strings.NewReader(`{}`))
	if r.StatusCode != http.StatusConflict {
		t.Fatalf("设备错误应 409: %d", r.StatusCode)
	}
	r.Body.Close()
}

func TestStatusAPI(t *testing.T) {
	srv := newCtlServer(t, &fakeCtl{on: true})
	body, _ := getBody(t, srv.URL+"/api/status")
	if !strings.Contains(body, `"micOn":true`) {
		t.Fatalf("状态应反映当前麦克风: %s", body)
	}
}
