// Package mcp 实现灵矽端侧 MCP 的协议语义（JSON-RPC 2.0 分发），
// 不含任何业务，也不关心传输来源（灵矽 WebSocket / 本地 CLI / 未来的云端 SSE）。
package mcp

import "encoding/json"

// JSONRPCVersion 灵矽端侧 MCP 使用标准 JSON-RPC 2.0。
const JSONRPCVersion = "2.0"

// Request 一条 JSON-RPC 请求（initialize / tools/list / tools/call）。
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response JSON-RPC 响应，result 与 error 互斥。
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError JSON-RPC 错误对象。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC 标准错误码。
const (
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

// ToolDescriptor 描述一个可被智能体调用的工具（tools/list 返回项）。
// Description 的质量直接决定智能体调用是否准确。
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// CallResult tools/call 的结果。
type CallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content 结果内容块；端侧目前只用 text。
type Content struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// TextResult 构造纯文本成功结果。
func TextResult(text string) *CallResult {
	return &CallResult{Content: []Content{{Type: "text", Text: text}}}
}

// ErrResult 构造业务错误结果（协议层成功、工具执行失败）。
func ErrResult(msg string) *CallResult {
	return &CallResult{Content: []Content{{Type: "text", Text: msg}}, IsError: true}
}
