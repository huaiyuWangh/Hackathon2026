// Package ccrunner 驱动 tmux 会话中的交互式 Claude Code，是全项目的核心。
// tmux 会话是唯一"现场"：语音链路（Part 1）经 Send 写入；
// 投屏（Part 2 方案A）直接镜像该终端所在屏幕，方案B（迭代）从 Output() 订阅。
package ccrunner

import "context"

// 任务状态。
const (
	TaskIdle    = "idle"    // 会话存在，无任务
	TaskRunning = "running" // 指令已下发，CC 执行中
	TaskDone    = "done"    // 最近一次任务成功结束
	TaskFailed  = "failed"  // 最近一次任务失败/被打断
)

// TaskStatus 一个 CC 任务的状态快照。
type TaskStatus struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Summary string `json:"summary"`
}

// Chunk 一段增量输出（来自 tmux capture-pane，保留 ANSI 色彩）。
type Chunk struct {
	Data []byte
}

// Runner 驱动交互式 Claude Code 的运行时接口。
type Runner interface {
	// Ensure 确保 tmux 会话与其中交互式 claude 进程已就绪。
	Ensure(ctx context.Context) error
	// Send 非阻塞下发一条指令（tmux send-keys），立即返回任务ID。
	Send(prompt string) (taskID string, err error)
	// Status 查询任务状态；taskID 为空表示最近一个任务。
	Status(taskID string) (TaskStatus, error)
	// Interrupt 打断当前任务（Esc）。
	Interrupt() error
	// Output 返回输出增量流，供查看器订阅；方案B迭代时启用。
	Output() <-chan Chunk
}
