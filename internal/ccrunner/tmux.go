package ccrunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TmuxRunner 通过 tmux 驱动交互式 Claude Code。
// tmux 会话是唯一"现场"：Send 用 send-keys 写入，AirPlay 直接镜像该终端，
// 方案B（迭代）从 Output() 订阅增量。
//
// 完成检测是启发式的（M1）：Claude Code 工作时页脚显示 "Esc to interrupt"，
// 据此判定 busy；标记消失且画面连续多个 tick 无变化则视为任务结束。
//
// 任务状态持久化在工作目录 .bridge/state.json：send/status 可能来自不同进程
// （--local 一次性 CLI），靠文件共享状态并各推进一拍状态机。
type TmuxRunner struct {
	session string
	workDir string
	claude  string // claude 可执行文件路径

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once

	mu           sync.Mutex
	seq          int
	tasks        map[string]*task
	last         *task
	lastPlain    string // 最近一次捕获的纯文本，供运行中任务摘要
	lastANSI     string // 最近一次捕获的 ANSI 画面（稳定性基线 + viewer 回放）
	lastSampleAt time.Time
	paneCols     int // tmux pane 尺寸（viewer 渲染对齐）
	paneRows     int
	subs         map[int64]chan Chunk // viewer 订阅者
	subSeq       int64
}

type task struct {
	id        string
	prompt    string
	state     string
	summary   string
	started   time.Time
	ended     time.Time
	sawBusy   bool // 是否出现过 busy 标记（区分刚下发 vs 已响应完）
	stableCnt int  // 连续无变化的采样数
}

// 状态机节奏参数。
const (
	tick             = 500 * time.Millisecond
	stableTicks      = 4                       // 连续无变化采样视为画面稳定 (~2s)
	minElapsedSeen   = 1500 * time.Millisecond // 见过 busy 后的最短判定等待
	minElapsedUnseen = 3 * time.Second         // 从未见到 busy 的最短判定等待
	bootTimeout      = 15 * time.Second        // claude 启动等待上限
	uiTimeout        = 10 * time.Second        // TUI 输入就绪探测上限
	busyMarker       = "esc to interrupt"      // CC 工作中的页脚标记（小写匹配）
	inputPrefix      = "❯"                     // CC 输入框行前缀
)

// NewTmux 创建 tmux 驱动的 Runner；检查 tmux/claude 可用性并装载历史状态。
func NewTmux(session, workDir string) (*TmuxRunner, error) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("ccrunner: 未找到 claude 命令, 请先安装 Claude Code CLI: %w", err)
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("ccrunner: 未找到 tmux, 请先 brew install tmux: %w", err)
	}
	workDir, err = absDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("ccrunner: 工作目录不可用: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &TmuxRunner{
		session: session,
		workDir: workDir,
		claude:  claude,
		ctx:     ctx,
		cancel:  cancel,
		tasks:   make(map[string]*task),
		subs:    make(map[int64]chan Chunk),
	}
	r.loadState()
	return r, nil
}

// Close 停止后台监控；进程退出场景可不调用。
func (r *TmuxRunner) Close() { r.cancel() }

// Ensure 确保 tmux 会话与其中交互式 claude 已就绪；不存在则创建并等启动。
func (r *TmuxRunner) Ensure(ctx context.Context) error {
	r.startMonitor()
	if r.hasSession(ctx) {
		return nil
	}
	if err := tmuxRun(ctx, "new-session", "-d", "-s", r.session, "-c", r.workDir, r.claude); err != nil {
		return fmt.Errorf("ccrunner: 创建 tmux 会话失败: %w", err)
	}
	if err := r.waitBoot(ctx); err != nil {
		return err
	}
	return r.probeUI(ctx)
}

// probeUI 用可回滚的按键往返探测输入就绪：键入一个字符，等它回显在输入框里
// 再退格删除。只等 "❯" 出现不够——欢迎屏的输入框先于 Ink 输入处理就绪，
// 提前 send-keys 会被吞掉（M1/M2 冒烟均踩坑）。仅新建会话时探测，
// 之后的 fast-path 不打扰用户在 attach 终端里的输入。

// waitBoot 等 pane 进程稳定为 node/claude（连续两次采样一致），再留 1s 渲染。
func (r *TmuxRunner) waitBoot(ctx context.Context) error {
	prev := ""
	stable := 0
	deadline := time.Now().Add(bootTimeout)
	for time.Now().Before(deadline) {
		out, err := tmuxOut(ctx, "display-message", "-p", "-t", r.session, "#{pane_current_command}")
		if err == nil {
			c := strings.TrimSpace(out)
			if (c == "node" || c == "claude") && c == prev {
				stable++
				if stable >= 2 {
					time.Sleep(1 * time.Second)
					return nil
				}
			} else {
				stable = 0
			}
			prev = c
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.ctx.Done():
			return context.Canceled
		case <-time.After(tick):
		}
	}
	return fmt.Errorf("ccrunner: claude 启动超时 (会话 %s), 可 tmux attach -t %s 查看现场", r.session, r.session)
}

// probeUI 实现 见 Ensure 内注释。
func (r *TmuxRunner) probeUI(ctx context.Context) error {
	if err := tmuxRun(ctx, "send-keys", "-t", r.session, "-l", "x"); err != nil {
		return fmt.Errorf("ccrunner: 探测按键发送失败: %w", err)
	}
	deadline := time.Now().Add(uiTimeout)
	for time.Now().Before(deadline) {
		out, err := tmuxOut(ctx, "capture-pane", "-p", "-t", r.session)
		if err == nil && probeEchoed(out) {
			_ = tmuxRun(ctx, "send-keys", "-t", r.session, "BSpace")
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.ctx.Done():
			return context.Canceled
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("ccrunner: claude 输入未就绪 (探测无回显), 可 tmux attach -t %s 查看现场", r.session)
}

// probeEchoed 判断探测字符是否已回显在输入框行（❯ ... x）。
func probeEchoed(pane string) bool {
	for _, line := range strings.Split(pane, "\n") {
		t := strings.TrimRight(line, " ")
		if len(t) > len(inputPrefix) && strings.HasPrefix(t, inputPrefix) && strings.HasSuffix(t, "x") {
			return true
		}
	}
	return false
}

// Send 非阻塞下发指令：必要时先 Ensure，然后 send-keys 文本 + Enter。
// 多行/制表符会被压成空格，避免 TUI 提前提交或错乱。
func (r *TmuxRunner) Send(prompt string) (string, error) {
	one := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(strings.TrimSpace(prompt))
	if one == "" {
		return "", fmt.Errorf("ccrunner: 空 prompt")
	}
	if err := r.Ensure(context.Background()); err != nil {
		return "", err
	}
	if err := tmuxRun(context.Background(), "send-keys", "-t", r.session, "-l", "--", one); err != nil {
		return "", fmt.Errorf("ccrunner: send-keys 失败: %w", err)
	}
	if err := tmuxRun(context.Background(), "send-keys", "-t", r.session, "Enter"); err != nil {
		return "", fmt.Errorf("ccrunner: 发送 Enter 失败: %w", err)
	}
	id := r.newTask(one)
	r.saveState()
	return id, nil
}

// Status 查询任务；taskID 为空表示最近一个任务，无任何任务返回 idle。
// 有运行中任务时先同步采样一拍，支持一次性 CLI 进程跨调用推进状态机。
func (r *TmuxRunner) Status(taskID string) (TaskStatus, error) {
	r.mu.Lock()
	running := r.last != nil && r.last.state == TaskRunning
	r.mu.Unlock()
	if running {
		r.syncSample()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var t *task
	if taskID == "" {
		t = r.last
	} else {
		var ok bool
		t, ok = r.tasks[taskID]
		if !ok {
			return TaskStatus{}, fmt.Errorf("ccrunner: 未知任务 %q", taskID)
		}
	}
	if t == nil {
		return TaskStatus{State: TaskIdle, Summary: "会话无任务"}, nil
	}
	summary := t.summary
	if t.state == TaskRunning {
		summary = "执行中 " + time.Since(t.started).Round(time.Second).String() + " · " + summarize(r.lastPlain)
	}
	return TaskStatus{ID: t.id, State: t.state, Summary: summary}, nil
}

// Interrupt 发送 Esc 打断当前任务。
func (r *TmuxRunner) Interrupt() error {
	if !r.hasSession(context.Background()) {
		return fmt.Errorf("ccrunner: tmux 会话 %s 不存在, 无任务可打断", r.session)
	}
	if err := tmuxRun(context.Background(), "send-keys", "-t", r.session, "Escape"); err != nil {
		return fmt.Errorf("ccrunner: 发送 Esc 失败: %w", err)
	}
	r.mu.Lock()
	if r.last != nil && r.last.state == TaskRunning {
		r.last.state = TaskFailed
		r.last.summary = "已打断 (Esc)"
		r.last.ended = time.Now()
	}
	r.mu.Unlock()
	r.saveState()
	return nil
}

func (r *TmuxRunner) newTask(prompt string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	t := &task{id: fmt.Sprintf("t%d", r.seq), prompt: prompt, state: TaskRunning, started: time.Now()}
	if r.last != nil && r.last.state == TaskRunning {
		r.last.state = TaskFailed
		r.last.summary = "被新任务 " + t.id + " 覆盖"
		r.last.ended = time.Now()
	}
	r.tasks[t.id] = t
	r.last = t
	return t.id
}

func (r *TmuxRunner) hasSession(ctx context.Context) bool {
	return tmuxRun(ctx, "has-session", "-t", r.session) == nil
}

func (r *TmuxRunner) capture(ansi bool) (string, error) {
	args := []string{"capture-pane", "-p", "-t", r.session}
	if ansi {
		args = append(args, "-e")
	}
	out, err := tmuxOut(context.Background(), args...)
	if err != nil {
		return "", err
	}
	return normalizePane(out), nil
}

// normalizePane 归一化 pane 捕获：每行去掉补齐用的尾部空格、去掉末尾空行。
// capture-pane 按行补满整宽，整行宽文本+\n 写入 xterm 会触发自动换行，
// 造成"每行多一空行"的显示混乱（viewer 实测踩坑）；回放与增量共用本归一化。
func normalizePane(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	if end == 0 {
		return ""
	}
	return strings.Join(lines[:end], "\n") + "\n"
}

func tmuxRun(ctx context.Context, args ...string) error {
	return exec.CommandContext(ctx, "tmux", args...).Run()
}

func tmuxOut(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	return string(out), err
}

// absDir 把相对路径解析为绝对路径（tmux -c 与状态文件路径需要确定性）。
func absDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, dir), nil
}
