package ccrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeltaOf(t *testing.T) {
	cases := []struct {
		prev, cur, want string
	}{
		{"", "abc", "abc"},
		{"abc", "abc", ""},
		{"abc", "abcdef", "def"},
		{"xyz", "abc", "\fabc"},
		{"abc", "", ""},
	}
	for _, c := range cases {
		if got := deltaOf(c.prev, c.cur); got != c.want {
			t.Errorf("deltaOf(%q,%q) = %q, want %q", c.prev, c.cur, got, c.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	if got := summarize("a\n\nb   \n\nc\n"); got != "a / b / c" {
		t.Errorf("summarize 基本拼接失败: %q", got)
	}
	if got := summarize("\n \n"); got != "(无输出)" {
		t.Errorf("空画面应为 (无输出): %q", got)
	}
	if got := summarize(strings.Repeat("x", 300)); len([]rune(got)) > 241 {
		t.Errorf("长输出未截断: %d", len([]rune(got)))
	}
}

func TestSummarizeSkipsChrome(t *testing.T) {
	pane := "" +
		"╭─── Claude Code v2.1.88 ────────────────╮\n" +
		"│            Welcome back!            │\n" +
		"               ▐▛███▜▌\n" +
		"   glm-5.3[1m] · API Usage Billing\n" +
		"    /private/tmp/bridge-cc-smoke\n" +
		"❯ 只回复两个字: ok\n" +
		"⏺ ok\n" +
		"──────────────────────────────\n" +
		"❯\n" +
		"  ⏵⏵ bypass permissions on     installMethod is native…\n" +
		"✗ Auto-update failed · Try claude doctor\n"
	got := summarize(pane)
	if !strings.Contains(got, "⏺ ok") || !strings.Contains(got, "只回复两个字: ok") {
		t.Errorf("摘要应保留对话内容: %q", got)
	}
	for _, bad := range []string{"bypass", "Auto-update", "Welcome", "────", "❯ /"} {
		if strings.Contains(got, bad) {
			t.Errorf("摘要不应包含 UI 装饰 %q: %q", bad, got)
		}
	}
}

func TestStepBusyThenStableDone(t *testing.T) {
	r := &TmuxRunner{}
	tk := &task{id: "t1", state: TaskRunning, started: time.Now().Add(-5 * time.Second)}
	r.last = tk

	// busy 标记存在 → 保持 running
	r.step("a", "b", "xxx Esc to interrupt yyy", tick)
	if tk.state != TaskRunning {
		t.Fatalf("busy 中应保持 running, 得到 %s", tk.state)
	}
	if !tk.sawBusy {
		t.Fatal("应记录 sawBusy")
	}

	// busy 消失 + 连续 4 tick 画面稳定 → done, 摘要含末行内容
	same := "stable frame"
	for i := 0; i < stableTicks; i++ {
		r.step(same, same, "回答: ok", tick)
	}
	if tk.state != TaskDone {
		t.Fatalf("稳定后应 done, 得到 %s", tk.state)
	}
	if !strings.Contains(tk.summary, "回答: ok") {
		t.Fatalf("摘要应含末行内容: %q", tk.summary)
	}
}

func TestStepTooEarlyStaysRunning(t *testing.T) {
	r := &TmuxRunner{}
	// 刚下发 1s、从未见过 busy：即使稳定也不得提前判定 done
	tk := &task{id: "t1", state: TaskRunning, started: time.Now().Add(-1 * time.Second)}
	r.last = tk
	same := "frame"
	for i := 0; i < stableTicks; i++ {
		r.step(same, same, same, tick)
	}
	if tk.state != TaskRunning {
		t.Fatalf("未到最短等待不应 done, 得到 %s", tk.state)
	}
}

func TestStepCrossProcessLongGap(t *testing.T) {
	r := &TmuxRunner{}
	// 跨进程场景：距上次采样隔了 3s 且画面未变 → 一次采样即视为稳定
	tk := &task{id: "t1", state: TaskRunning, started: time.Now().Add(-5 * time.Second)}
	r.last = tk
	same := "frame"
	r.step(same, same, "答案: 完成", 3*time.Second)
	if tk.state != TaskDone {
		t.Fatalf("长间隔稳定采样应 done, 得到 %s", tk.state)
	}
}

func TestProbeEchoed(t *testing.T) {
	if !probeEchoed("────\n❯\n────\n❯ x\n") {
		t.Fatal("探测字符已回显应返回 true")
	}
	if !probeEchoed("❯ abc x") {
		t.Fatal("输入框已有内容时应识别末尾探测字符")
	}
	if probeEchoed("❯\nWelcome back!\n") {
		t.Fatal("未回显应返回 false")
	}
}

func TestNormalizePane(t *testing.T) {
	in := "hello     \nworld  \n\n\n"
	got := normalizePane(in)
	if got != "hello\nworld\n" {
		t.Fatalf("归一化结果异常: %q", got)
	}
	if got := normalizePane("\n\n"); got != "" {
		t.Fatalf("全空画面应为空: %q", got)
	}
	// 保留行内有意义的内容与 ANSI
	in2 := "\x1b[32m绿\x1b[0m  \ntail"
	if got := normalizePane(in2); got != "\x1b[32m绿\x1b[0m\ntail\n" {
		t.Fatalf("ANSI/内容不应被破坏: %q", got)
	}
}

func TestFailRunning(t *testing.T) {
	r := newBareRunner(t)
	tk := &task{id: "t1", state: TaskRunning, started: time.Now()}
	r.last = tk
	r.failRunning("boom")
	if tk.state != TaskFailed || tk.summary != "boom" {
		t.Fatalf("failRunning 生效失败: %+v", tk)
	}
}

func TestStatusIdleAndUnknown(t *testing.T) {
	r := newBareRunner(t)
	st, err := r.Status("")
	if err != nil || st.State != TaskIdle {
		t.Fatalf("无任务应返回 idle: %+v %v", st, err)
	}
	if _, err := r.Status("t99"); err == nil {
		t.Fatal("未知任务应报错")
	}
}

func TestAbsDir(t *testing.T) {
	wd, _ := os.Getwd()
	for dir, want := range map[string]string{
		"":       wd,
		".":      wd,
		"sub":    filepath.Join(wd, "sub"),
		"/abs/x": "/abs/x",
	} {
		got, err := absDir(dir)
		if err != nil || got != want {
			t.Errorf("absDir(%q) = %q, %v; want %q", dir, got, err, want)
		}
	}
}

func TestNewTaskSupersedesRunning(t *testing.T) {
	r := newBareRunner(t)
	id1 := r.newTask("第一条")
	id2 := r.newTask("第二条")
	if r.tasks[id1].state != TaskFailed || !strings.Contains(r.tasks[id1].summary, "覆盖") {
		t.Fatalf("旧任务应被覆盖判失败: %+v", r.tasks[id1])
	}
	if r.last == nil || r.last.id != id2 || r.last.state != TaskRunning {
		t.Fatalf("新任务应为 running 的 last: %+v", r.last)
	}
}

func TestSendEmptyPrompt(t *testing.T) {
	r := newBareRunner(t)
	for _, p := range []string{"", "  ", "\n\t "} {
		if _, err := r.Send(p); err == nil || !strings.Contains(err.Error(), "空 prompt") {
			t.Errorf("空 prompt 应报错: %q → %v", p, err)
		}
	}
}

func TestInterruptWithoutSession(t *testing.T) {
	r := newBareRunner(t) // session 名不存在的会话
	if err := r.Interrupt(); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("无会话时打断应报错: %v", err)
	}
}

func TestPublishAndSubscribe(t *testing.T) {
	r := newBareRunner(t)
	v := r.Subscribe() // 冷启动：capture 失败走 failRunning, 不 panic
	if v.Ch == nil || v.Cancel == nil {
		t.Fatalf("订阅应返回增量流与取消函数: %+v", v)
	}

	// 全量重绘增量应广播给订阅者；无增量不广播（慢消费者语义）
	r.publish("ansi1", "plain1", "\fansi1")
	select {
	case c := <-v.Ch:
		if string(c.Data) != "\fansi1" {
			t.Fatalf("增量内容异常: %q", c.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("增量未到达")
	}
	r.publish("ansi1", "plain1", "")
	select {
	case c := <-v.Ch:
		t.Fatalf("无增量不应广播: %q", c.Data)
	default:
	}

	// 取消订阅后不再接收
	v.Cancel()
	r.publish("ansi2", "plain2", "\fansi2")
	select {
	case c := <-v.Ch:
		t.Fatalf("取消后不应再收到: %q", c.Data)
	default:
	}
}

func TestMonitorLoopRunsAndStops(t *testing.T) {
	r := newBareRunner(t)
	go r.monitorLoop()
	time.Sleep(1200 * time.Millisecond) // ≥2 个 tick: capture 失败路径反复执行
	r.cancel()                          // ctx 取消后循环应退出（不 panic、不死锁）
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(r.statePath()); err != nil {
		t.Fatalf("监控循环应持久化状态: %v", err)
	}
}

func TestNewTmuxRequiresClaudeAndTmux(t *testing.T) {
	if _, err := exec.LookPath("claude"); err == nil {
		t.Skip("本机已装 claude, 缺依赖分支在 CI 上验证")
	}
	if _, err := NewTmux("s", t.TempDir()); err == nil || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("缺 claude 应报错: %v", err)
	}
}
