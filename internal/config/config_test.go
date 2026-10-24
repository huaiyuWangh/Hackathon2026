package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("# 注释\nFOO=bar\nQUX=\"z z\"\nEMPTY=\nBADLINE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOO", "preset") // 已设置的环境变量优先, 不被 .env 覆盖
	loadDotEnv(p)
	if os.Getenv("FOO") != "preset" {
		t.Errorf("FOO 应保持 preset: %s", os.Getenv("FOO"))
	}
	if os.Getenv("QUX") != "z z" {
		t.Errorf("QUX 引号应被剥掉: %q", os.Getenv("QUX"))
	}
	if os.Getenv("EMPTY") != "" {
		t.Errorf("空值应保持空: %q", os.Getenv("EMPTY"))
	}
}

func TestClientIDPersist(t *testing.T) {
	dir := t.TempDir()
	id1 := clientID(dir)
	id2 := clientID(dir)
	if id1 != id2 || len(id1) != 32 {
		t.Fatalf("Client-Id 应持久化且为 32 位 hex: %q vs %q", id1, id2)
	}
}

func TestDetectMAC(t *testing.T) {
	if m := detectMAC(); m == "" {
		t.Fatal("MAC 探测不应返回空")
	}
	if strings.Contains(detectMAC(), " ") {
		t.Fatal("MAC 不应含空格")
	}
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	// 全部留空 → 默认值；ClientID 落在临时 workDir，不污染包目录
	t.Setenv("BRIDGE_WORK_DIR", t.TempDir())
	for _, k := range []string{"BRIDGE_LINX_WS_URL", "BRIDGE_DEVICE_ID", "BRIDGE_API_KEY",
		"BRIDGE_TMUX_SESSION", "BRIDGE_MCP_PROXY_BASE", "BRIDGE_SSE_ADDR",
		"BRIDGE_VIEWER_ADDR", "BRIDGE_AUDIO", "BRIDGE_DEBUG"} {
		t.Setenv(k, "")
	}
	cfg := Load()
	if cfg.LinxWSURL != "wss://xrobo-io.qiniuapi.com/v1/ws/" || cfg.TmuxSession != "ccline" ||
		cfg.SSEAddr != "127.0.0.1:8787" || cfg.ViewerAddr != "0.0.0.0:8788" ||
		cfg.McpProxyBase != "https://xrobo.qiniu.com" || cfg.WorkDir == "." {
		t.Fatalf("默认值异常: %+v", cfg)
	}
	if cfg.DeviceID == "" || cfg.ClientID == "" {
		t.Fatalf("DeviceID/ClientID 应自动生成: %+v", cfg)
	}
	if cfg.Audio || cfg.Debug {
		t.Fatal("BRIDGE_AUDIO/DEBUG 留空应为 false")
	}

	// 环境变量覆盖全部生效
	t.Setenv("BRIDGE_LINX_WS_URL", "ws://x")
	t.Setenv("BRIDGE_API_KEY", "k")
	t.Setenv("BRIDGE_AUDIO", "1")
	t.Setenv("BRIDGE_TMUX_SESSION", "s2")
	cfg = Load()
	if cfg.LinxWSURL != "ws://x" || cfg.APIKey != "k" || !cfg.Audio || cfg.TmuxSession != "s2" {
		t.Fatalf("覆盖未生效: %+v", cfg)
	}
}
