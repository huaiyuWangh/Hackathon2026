// Package linx 实现灵矽设备接入协议的客户端（传输层，无业务）。
package linx

import "encoding/json"

// 灵矽设备接入协议关键消息（承载于设备 WebSocket）。
// 完整格式与联调结论记在 docs/linx-protocol-notes.md，M3 联调时补齐。

// Envelope 设备消息外层，type 决定 payload 语义。
// 已知类型：hello（设备上线通告）、mcp（端侧 MCP JSON-RPC）、
// audio（Opus 帧，通常为二进制帧）、state（listen_state/tts_state 等）。
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Hello 设备连接后的首条消息，声明能力；features.mcp=true 平台才会下发 MCP 消息。
type Hello struct {
	Type     string   `json:"type"` // "hello"
	DeviceID string   `json:"device_id"`
	Features Features `json:"features"`
}

// Features 设备能力开关。
type Features struct {
	MCP bool `json:"mcp"`
}
