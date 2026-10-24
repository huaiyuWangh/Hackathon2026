package ccrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newBareRunner 构造不依赖 tmux 的最小 TmuxRunner（workDir 指向临时目录，
// 防止状态文件泄漏到包目录——历史上真实发生过）。
func newBareRunner(t *testing.T) *TmuxRunner {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &TmuxRunner{
		session: "vc-test-no-such",
		workDir: t.TempDir(),
		ctx:     ctx,
		cancel:  cancel,
		tasks:   make(map[string]*task),
		subs:    make(map[int64]chan Chunk),
	}
}

func TestStateRoundTrip(t *testing.T) {
	r := newBareRunner(t)
	base := time.Now().Add(-time.Minute).Truncate(time.Second)
	r.seq = 3
	r.lastANSI, r.lastPlain, r.lastSampleAt = "\x1b[32mansi\x1b[0m\n", "plain\n", base
	r.tasks["t1"] = &task{id: "t1", prompt: "跑测试", state: TaskDone, summary: "ok",
		started: base, ended: base.Add(time.Second)}
	r.tasks["t2"] = &task{id: "t2", prompt: "新任务", state: TaskRunning, started: base}
	r.last = r.tasks["t2"]
	r.saveState()

	if _, err := os.Stat(filepath.Join(r.workDir, ".bridge", "state.json")); err != nil {
		t.Fatalf("状态文件应写入 workDir/.bridge: %v", err)
	}

	// 跨进程共享：新实例从同一 workDir 装载应还原全部字段
	r2 := newBareRunner(t)
	r2.workDir = r.workDir
	r2.loadState()
	if r2.seq != 3 || r2.lastANSI != "\x1b[32mansi\x1b[0m\n" || r2.lastPlain != "plain\n" {
		t.Fatalf("基础字段未还原: seq=%d ansi=%q", r2.seq, r2.lastANSI)
	}
	if !r2.lastSampleAt.Equal(base) {
		t.Fatalf("采样时刻未还原: %v vs %v", r2.lastSampleAt, base)
	}
	t2 := r2.tasks["t2"]
	if t2 == nil || t2.state != TaskRunning || !t2.started.Equal(base) {
		t.Fatalf("任务未还原: %+v", t2)
	}
	if r2.last == nil || r2.last.id != "t2" {
		t.Fatalf("last 指针未还原: %+v", r2.last)
	}
	if r2.tasks["t1"].state != TaskDone || r2.tasks["t1"].summary != "ok" {
		t.Fatalf("完成任务字段未还原: %+v", r2.tasks["t1"])
	}
}

func TestLoadStateToleratesBadFile(t *testing.T) {
	r := newBareRunner(t)
	// 损坏内容：静默从零开始
	if err := os.MkdirAll(filepath.Join(r.workDir, ".bridge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.statePath(), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.loadState()
	if r.seq != 0 || len(r.tasks) != 0 || r.last != nil {
		t.Fatalf("损坏状态应从零开始: seq=%d tasks=%d", r.seq, len(r.tasks))
	}

	// 文件缺失：同样从零开始
	r2 := newBareRunner(t)
	r2.loadState()
	if r2.seq != 0 || r2.lastANSI != "" {
		t.Fatal("缺文件应从零开始")
	}
}
