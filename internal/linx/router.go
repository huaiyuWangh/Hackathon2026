package linx

import (
	"context"
	"encoding/json"

	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

// Router 把设备连接上收到的外层消息路由给对应处理器。
// M2 实现纯编解码与 MCP 分发，可脱离 WebSocket 单测/本地模拟（bridge --local sim）；
// M3 的 WebSocket 收发循环只负责 IO：收到字节流交给 HandleEnvelope，返回值回写。
type Router struct {
	mcpSrv *mcp.Server
}

// NewRouter 用工具集合构造路由。
func NewRouter(p mcp.ToolProvider) *Router {
	return &Router{mcpSrv: mcp.NewServer(p)}
}

// HandleEnvelope 处理一条外层消息（如 {"type":"mcp","payload":{...}}），
// 返回需要回写的完整消息；nil 表示无需响应。
func (r *Router) HandleEnvelope(data []byte) []byte {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		// 外层不是合法 JSON，无法关联请求，只能丢弃
		return nil
	}
	if env.Type != "mcp" {
		// stt/llm/tts/notify 等服务端下行：纯 MCP 设备忽略，不得影响连接
		return nil
	}
	var req mcp.Request
	if err := json.Unmarshal(env.Payload, &req); err != nil || req.JSONRPC == "" {
		return r.wrap(&mcp.Response{
			JSONRPC: mcp.JSONRPCVersion,
			Error:   &mcp.RPCError{Code: -32700, Message: "linx: MCP payload 不是合法的 JSON-RPC 请求"},
		})
	}
	resp := r.mcpSrv.Handle(context.Background(), &req)
	if len(req.ID) == 0 {
		return nil // 通知：已执行，不回包
	}
	return r.wrap(resp)
}

func (r *Router) wrap(resp *mcp.Response) []byte {
	payload, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	out, err := json.Marshal(Envelope{Type: "mcp", Payload: payload})
	if err != nil {
		return nil
	}
	return out
}
