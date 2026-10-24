package brand

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeIcon(t *testing.T) {
	if Name != "VoiceCopilot" {
		t.Fatalf("品牌名异常: %q", Name)
	}
	w := httptest.NewRecorder()
	ServeIcon(w, httptest.NewRequest("GET", "/icon.svg", nil))
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "svg") {
		t.Fatalf("图标应 200 svg: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "<svg") {
		t.Fatal("图标内容应为 SVG")
	}
}
