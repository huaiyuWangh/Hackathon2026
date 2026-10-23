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
