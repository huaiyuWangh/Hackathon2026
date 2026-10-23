package tools

import (
	"context"
	"encoding/json"

	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

// registerClaudeTools 注册语音驱动 Claude Code 的工具集。
// 设计原则：send 非阻塞（长任务）、status 供智能体轮询播报、interrupt 兜底。
func (reg *Registry) registerClaudeTools() {
	reg.register(mcp.ToolDescriptor{
		Name:        "dev.claude.send",
		Description: "向 Claude Code 下发一条编程指令（自然语言），立即返回任务ID，不等待完成。之后用 dev.claude.status 查询进度。",
		InputSchema: schema(map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "编程任务描述，例如：修复 utils_test.go 里失败的登录用例",
			},
		}, "prompt"),
	}, reg.handleClaudeSend)

	reg.register(mcp.ToolDescriptor{
		Name:        "dev.claude.status",
		Description: "查询 Claude Code 任务状态与最新摘要（running/done/failed），用于向用户语音播报进展。",
		InputSchema: schema(map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "dev.claude.send 返回的任务ID；省略时查最近一个任务",
			},
		}),
	}, reg.handleClaudeStatus)

	reg.register(mcp.ToolDescriptor{
		Name:        "dev.claude.interrupt",
		Description: "打断当前正在执行的 Claude Code 任务（相当于按 Esc）。危险操作确认前可先打断。",
		InputSchema: schema(map[string]any{}),
	}, reg.handleClaudeInterrupt)
}

func (reg *Registry) handleClaudeSend(ctx context.Context, args json.RawMessage) (*mcp.CallResult, error) {
	var in struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return mcp.ErrResult("参数解析失败: " + err.Error()), nil
	}
	if in.Prompt == "" {
		return mcp.ErrResult("prompt 不能为空"), nil
	}
	id, err := reg.runner.Send(in.Prompt)
	if err != nil {
		return nil, err
	}
	return mcp.TextResult("已接收, task=" + id + "。请稍后用 dev.claude.status 查询进度。"), nil
}

func (reg *Registry) handleClaudeStatus(ctx context.Context, args json.RawMessage) (*mcp.CallResult, error) {
	var in struct {
		Task string `json:"task"`
	}
	_ = json.Unmarshal(args, &in)
	st, err := reg.runner.Status(in.Task)
	if err != nil {
		return nil, err
	}
	return mcp.TextResult(st.State + " | " + st.Summary), nil
}

func (reg *Registry) handleClaudeInterrupt(ctx context.Context, args json.RawMessage) (*mcp.CallResult, error) {
	if err := reg.runner.Interrupt(); err != nil {
		return nil, err
	}
	return mcp.TextResult("已发送打断信号"), nil
}
