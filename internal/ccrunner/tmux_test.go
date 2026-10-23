package ccrunner

import (
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

func TestFailRunning(t *testing.T) {
	r := &TmuxRunner{}
	tk := &task{id: "t1", state: TaskRunning, started: time.Now()}
	r.last = tk
	r.failRunning("boom")
	if tk.state != TaskFailed || tk.summary != "boom" {
		t.Fatalf("failRunning 生效失败: %+v", tk)
	}
}

func TestStatusIdleAndUnknown(t *testing.T) {
	r := &TmuxRunner{}
	st, err := r.Status("")
	if err != nil || st.State != TaskIdle {
		t.Fatalf("无任务应返回 idle: %+v %v", st, err)
	}
	if _, err := r.Status("t99"); err == nil {
		t.Fatal("未知任务应报错")
	}
}
