package mcp

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrUnknownTool 工具未注册；按灵矓文档映射为 JSON-RPC -32601。
var ErrUnknownTool = errors.New("mcp: 未知工具")

// ToolProvider 业务工具集合，由 internal/tools 实现。
// mcp 包只依赖此接口，保持协议层与业务层解耦。
type ToolProvider interface {
	Descriptors() []ToolDescriptor
	Call(ctx context.Context, name string, args json.RawMessage) (*CallResult, error)
}

// Server 把 MCP 请求分发给 ToolProvider。
type Server struct {
	provider ToolProvider
}

// NewServer 创建协议分发器。
func NewServer(p ToolProvider) *Server {
	return &Server{provider: p}
}

// Handle 处理一条 MCP 请求，永不返回 nil（协议错误也走 error 字段）。
func (s *Server) Handle(ctx context.Context, req *Request) *Response {
	switch req.Method {
	case "initialize":
		// protocolVersion 按平台联调结论调整（docs/linx-protocol-notes.md）。
		return ok(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "bridge-mac", "version": "0.0.1"},
		})
	case "tools/list":
		return ok(req.ID, map[string]any{"tools": s.provider.Descriptors()})
	case "tools/call":
		return s.handleCall(ctx, req)
	default:
		return errResp(req.ID, ErrMethodNotFound, "mcp: 未知方法 "+req.Method)
	}
}

func (s *Server) handleCall(ctx context.Context, req *Request) *Response {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		return errResp(req.ID, ErrInvalidParams, "mcp: tools/call 需要 {name, arguments}")
	}
	result, err := s.provider.Call(ctx, params.Name, params.Arguments)
	if err != nil {
		code := ErrInternal
		if errors.Is(err, ErrUnknownTool) {
			code = ErrMethodNotFound // 灵矓文档：未知工具返回 -32601
		}
		return errResp(req.ID, code, err.Error())
	}
	return ok(req.ID, result)
}

func ok(id json.RawMessage, result any) *Response {
	return &Response{JSONRPC: JSONRPCVersion, ID: id, Result: result}
}

func errResp(id json.RawMessage, code int, msg string) *Response {
	return &Response{JSONRPC: JSONRPCVersion, ID: id, Error: &RPCError{Code: code, Message: msg}}
}
