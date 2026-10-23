package linx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

type fakeProvider struct{}

func (fakeProvider) Descriptors() []mcp.ToolDescriptor {
	return []mcp.ToolDescriptor{{Name: "self.test.ping", Description: "测试工具"}}
}

func (fakeProvider) Call(ctx context.Context, name string, args json.RawMessage) (*mcp.CallResult, error) {
	if name == "nope" {
		return nil, mcp.ErrUnknownTool
	}
	return mcp.TextResult("pong"), nil
}

func TestRouterToolsList(t *testing.T) {
	r := NewRouter(fakeProvider{})
	out := r.HandleEnvelope([]byte(`{"type":"mcp","payload":{"jsonrpc":"2.0","id":7,"method":"tools/list"}}`))
	if out == nil {
		t.Fatal("tools/list 应有响应")
	}
	var env Envelope
	if err := json.Unmarshal(out, &env); err != nil || env.Type != "mcp" {
		t.Fatalf("响应外层应为 mcp 信封: %s", out)
	}
	if !strings.Contains(string(env.Payload), `"self.test.ping"`) {
		t.Fatalf("payload 应含工具描述: %s", env.Payload)
	}
	if !strings.Contains(string(env.Payload), `"id":7`) {
		t.Fatalf("响应应回带请求 id: %s", env.Payload)
	}
}

func TestRouterUnknownToolIs32601(t *testing.T) {
	r := NewRouter(fakeProvider{})
	out := r.HandleEnvelope([]byte(`{"type":"mcp","payload":{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"nope"}}}`))
	if out == nil {
		t.Fatal("未知工具应有错误响应")
	}
	if !strings.Contains(string(out), "-32601") {
		t.Fatalf("未知工具应映射 -32601: %s", out)
	}
}

func TestRouterNotificationNoResponse(t *testing.T) {
	r := NewRouter(fakeProvider{})
	out := r.HandleEnvelope([]byte(`{"type":"mcp","payload":{"jsonrpc":"2.0","method":"tools/list"}}`))
	if out != nil {
		t.Fatalf("通知不应回包: %s", out)
	}
}

func TestRouterNonMCPTypeIgnored(t *testing.T) {
	r := NewRouter(fakeProvider{})
	if out := r.HandleEnvelope([]byte(`{"type":"audio"}`)); out != nil {
		t.Fatalf("非 mcp 类型 M2 应忽略: %s", out)
	}
	if out := r.HandleEnvelope([]byte(`not json`)); out != nil {
		t.Fatalf("坏消息应丢弃: %s", out)
	}
}

func TestRouterBadPayload(t *testing.T) {
	r := NewRouter(fakeProvider{})
	// payload 是合法 JSON 但不是 JSON-RPC 请求对象
	out := r.HandleEnvelope([]byte(`{"type":"mcp","payload":123}`))
	if out == nil {
		t.Fatal("坏 payload 应回错误")
	}
	if !strings.Contains(string(out), "-32700") {
		t.Fatalf("坏 payload 应回 -32700: %s", out)
	}
}
