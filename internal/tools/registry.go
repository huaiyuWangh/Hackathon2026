// Package tools 实现语音驱动 Claude Code 的业务工具（dev.claude.*）。
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/huaiyuWangh/Hackathon2026/internal/ccrunner"
	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

// Handler 一个工具的业务实现。
type Handler func(ctx context.Context, args json.RawMessage) (*mcp.CallResult, error)

// Registry 实现 mcp.ToolProvider：汇集并分发业务工具。
type Registry struct {
	runner   ccrunner.Runner
	descs    []mcp.ToolDescriptor
	handlers map[string]Handler
}

// NewRegistry 用给定的 CC 运行时构造工具集。
func NewRegistry(r ccrunner.Runner) *Registry {
	reg := &Registry{
		runner:   r,
		handlers: make(map[string]Handler),
	}
	reg.registerClaudeTools()
	return reg
}

// Descriptors 返回全部工具描述（tools/list）。
func (reg *Registry) Descriptors() []mcp.ToolDescriptor { return reg.descs }

// Call 分发一次工具调用（tools/call）；未知工具返回错误。
func (reg *Registry) Call(ctx context.Context, name string, args json.RawMessage) (*mcp.CallResult, error) {
	h, ok := reg.handlers[name]
	if !ok {
		return nil, fmt.Errorf("tools: 未知工具 %q", name)
	}
	return h(ctx, args)
}

func (reg *Registry) register(d mcp.ToolDescriptor, h Handler) {
	reg.descs = append(reg.descs, d)
	reg.handlers[d.Name] = h
}

// schema 构造最简 JSON Schema。
func schema(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}
