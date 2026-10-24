package ccrunner

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// startMonitor 幂等启动捕获/状态机循环（常驻进程内使用）。
func (r *TmuxRunner) startMonitor() {
	r.once.Do(func() { go r.monitorLoop() })
}

func (r *TmuxRunner) monitorLoop() {
	prev := ""
	prevAt := time.Time{}
	first := true
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
		ansi, err := r.capture(true)
		if err != nil {
			r.failRunning("tmux 捕获失败: " + err.Error())
			continue
		}
		plain, err := r.capture(false)
		if err != nil {
			r.failRunning("tmux 捕获失败: " + err.Error())
			continue
		}
		age := time.Since(prevAt)
		if first {
			// 用持久化的上一次采样做基线，衔接跨进程的采样历史
			r.mu.Lock()
			base, baseAt := r.lastANSI, r.lastSampleAt
			r.mu.Unlock()
			if base != "" && !baseAt.IsZero() {
				prev, age = base, time.Since(baseAt)
			}
			first = false
		}

		r.publish(ansi, plain, deltaOf(prev, ansi))
		r.step(prev, ansi, plain, age)
		r.saveState()
		prev, prevAt = ansi, time.Now()
	}
}

// publish 原子地推进画面基线并广播增量：与 Subscribe 持同一把锁，
// 订阅者要么拿到旧回放+进入广播列表，要么拿到新回放且不在本次广播列表，
// 不会出现回放与增量重叠或缺失。
func (r *TmuxRunner) publish(ansi, plain, delta string) {
	r.mu.Lock()
	r.lastANSI = ansi
	r.lastPlain = plain
	r.lastSampleAt = time.Now()
	if cols, rows, ok := r.querySize(); ok {
		r.paneCols, r.paneRows = cols, rows
	}
	subs := make([]chan Chunk, 0, len(r.subs))
	for _, ch := range r.subs {
		subs = append(subs, ch)
	}
	r.mu.Unlock()

	if delta == "" {
		return
	}
	for _, ch := range subs {
		select {
		case ch <- Chunk{Data: []byte(delta)}:
		default: // 慢消费者丢帧，直播语义优先实时
		}
	}
}

// querySize 查询 tmux pane 尺寸（列/行），保证 viewer 终端渲染对齐。
func (r *TmuxRunner) querySize() (int, int, bool) {
	out, err := tmuxOut(context.Background(), "display-message", "-p", "-t", r.session, "#{pane_width} #{pane_height}")
	if err != nil {
		return 0, 0, false
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return 0, 0, false
	}
	cols, err1 := strconv.Atoi(fields[0])
	rows, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return cols, rows, true
}

// Subscribe 订阅现场画面：返回全量回放与增量流（viewer 用）。
// 冷启动（monitor 未跑过）时先同步采一拍，保证订阅者拿到真实尺寸与画面。
func (r *TmuxRunner) Subscribe() View {
	r.startMonitor()
	r.mu.Lock()
	cold := r.paneCols == 0 || r.lastANSI == ""
	r.mu.Unlock()
	if cold {
		r.syncSample()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subSeq++
	id := r.subSeq
	ch := make(chan Chunk, 128)
	if r.subs == nil {
		r.subs = make(map[int64]chan Chunk)
	}
	r.subs[id] = ch
	return View{
		Cols:   r.paneCols,
		Rows:   r.paneRows,
		Replay: r.lastANSI,
		Ch:     ch,
		Cancel: func() {
			r.mu.Lock()
			delete(r.subs, id)
			r.mu.Unlock()
		},
	}
}

// syncSample 一次性 CLI 进程（status 等）用：采样一拍并推进状态机。
func (r *TmuxRunner) syncSample() {
	ansi, err := r.capture(true)
	if err != nil {
		r.failRunning("tmux 捕获失败: " + err.Error())
		return
	}
	plain, err := r.capture(false)
	if err != nil {
		r.failRunning("tmux 捕获失败: " + err.Error())
		return
	}
	r.mu.Lock()
	prev, at := r.lastANSI, r.lastSampleAt
	r.mu.Unlock()
	r.publish(ansi, plain, deltaOf(prev, ansi))
	r.step(prev, ansi, plain, time.Since(at))
}

// step 单步状态机：画面稳定且无 busy 标记时结束运行中任务。纯逻辑，可单测。
// sampleAge 为距上一次采样的墙钟间隔：跨进程长间隔采样（画面未变）直接视为稳定。
func (r *TmuxRunner) step(prevANSI, ansi, plain string, sampleAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.last
	if t == nil || t.state != TaskRunning {
		return
	}
	if strings.Contains(strings.ToLower(plain), busyMarker) {
		t.sawBusy = true
		t.stableCnt = 0
		return
	}
	if ansi == prevANSI {
		if sampleAge >= tick*stableTicks {
			t.stableCnt = stableTicks
		} else {
			t.stableCnt++
		}
	} else {
		t.stableCnt = 0
	}
	elapsed := time.Since(t.started)
	minElapsed := minElapsedUnseen
	if t.sawBusy {
		minElapsed = minElapsedSeen
	}
	if t.stableCnt >= stableTicks && elapsed >= minElapsed {
		t.state = TaskDone
		t.ended = time.Now()
		t.summary = summarize(plain)
	}
}

// failRunning 将运行中任务标记为失败（会话丢失等致命场景）。
func (r *TmuxRunner) failRunning(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last != nil && r.last.state == TaskRunning {
		r.last.state = TaskFailed
		r.last.summary = msg
		r.last.ended = time.Now()
	}
	r.saveStateLocked()
}

// deltaOf 计算增量：新内容以旧内容为前缀则返回后缀；否则整体重绘，
// 前缀 \f 供消费者识别为清屏信号。
func deltaOf(prev, cur string) string {
	if cur == "" || cur == prev {
		return ""
	}
	if prev == "" {
		return cur
	}
	if strings.HasPrefix(cur, prev) {
		return cur[len(prev):]
	}
	return "\f" + cur
}

// summarize 取画面末尾最多 5 条非 UI 装饰的非空行，拼成可语音播报的摘要。
func summarize(plain string) string {
	lines := strings.Split(plain, "\n")
	var keep []string
	for i := len(lines) - 1; i >= 0 && len(keep) < 5; i-- {
		s := strings.TrimSpace(lines[i])
		if isChromeLine(s) {
			continue
		}
		keep = append([]string{s}, keep...)
	}
	s := strings.Join(keep, " / ")
	if s == "" {
		return "(无输出)"
	}
	if rs := []rune(s); len(rs) > 240 {
		s = "…" + string(rs[len(rs)-240:])
	}
	return s
}

// isChromeLine 判断一行是否为 CC 界面装饰（边框/块元素 logo/欢迎屏/页脚/空输入框），
// 摘要时跳过。启发式清单，随 CC 版本演进维护。
func isChromeLine(s string) bool {
	if s == "" || s == "❯" {
		return true
	}
	// 整行仅由制表符/块元素/空白构成的边框或 logo
	allDraw := true
	for _, r := range s {
		if !strings.ContainsRune("─━│┃┄┅┈═╌╭╮╰╯▁▂▃▄▅▆▇█▌▐░▒▓▀▄▝▜▛▙▟▚▏▖▗▘ ·", r) {
			allDraw = false
			break
		}
	}
	if allDraw {
		return true
	}
	// 去掉行首尾边框字符后再匹配欢迎屏/页脚文案
	trim := strings.Trim(s, "─━│┃┄┅┈═╌╭╮╰╯· ")
	for _, p := range chromePrefixes {
		if strings.HasPrefix(trim, p) {
			return true
		}
	}
	// 欢迎屏文案可能夹在边框/logo 之间，用包含匹配
	for _, c := range chromeContains {
		if strings.Contains(s, c) {
			return true
		}
	}
	// 纯路径行（欢迎屏的工作目录展示）
	if strings.HasPrefix(trim, "/") && !strings.Contains(trim, " ") {
		return true
	}
	return false
}

var chromePrefixes = []string{
	"Claude Code v",
	"⏵⏵",
	"bypass permissions on",
	"Auto-update failed",
	"✗ Auto-update",
	"Claude Code has switched",
	"installMethod is native",
	"· Esc to interrupt",
}

var chromeContains = []string{
	"Welcome back",
	"Welcome to",
	"Tips for getting started",
	"Run /init",
	"Recent activity",
	"No recent activity",
	"API Usage Billing",
}
