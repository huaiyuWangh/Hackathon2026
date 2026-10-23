package linx

import "encoding/json"

// 灵矽设备接入协议关键消息（承载于设备 WebSocket）。
// 依据 docs/linx-protocol-notes.md（官方文档摘录 + 联调结论）。

// Envelope 设备消息外层，type 决定 payload 语义。
// 已知类型：hello（握手）、mcp（端侧 MCP JSON-RPC）、
// stt / llm / tts / notify（服务端下行，纯 MCP 设备忽略）。
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Hello 设备连接后发送的首条消息。
// 注意：设备标识不在 hello 里，在 WebSocket 握手 header（Device-Id）中。
type Hello struct {
	Type        string         `json:"type"`    // "hello"
	Version     int            `json:"version"` // 须与 header Protocol-Version 一致
	Features    Features       `json:"features"`
	Transport   string         `json:"transport"`    // "websocket"
	AudioParams map[string]any `json:"audio_params"` // 纯 MCP 设备传空对象
}

// Features 设备能力开关。
type Features struct {
	MCP    bool `json:"mcp"`
	Notify bool `json:"notify,omitempty"` // 可选：接收 notify: config_updated
}

// ServerHello 服务端对 hello 的应答。
type ServerHello struct {
	Type        string         `json:"type"` // "hello"
	SessionID   string         `json:"session_id,omitempty"`
	AudioParams map[string]any `json:"audio_params,omitempty"`
}
