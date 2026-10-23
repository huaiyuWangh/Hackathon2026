package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeProvider struct{}

func (fakeProvider) Descriptors() []ToolDescriptor {
	return []ToolDescriptor{{Name: "self.test.ping", Description: "测试工具"}}
}

func (fakeProvider) Call(ctx context.Context, name string, args json.RawMessage) (*CallResult, error) {
	if name != "self.test.ping" {
		return nil, ErrUnknownTool
	}
	return TextResult("pong"), nil
}

func TestHandleUnknownMethod(t *testing.T) {
	resp := NewServer(fakeProvider{}).Handle(context.Background(), &Request{Method: "nope"})
	if resp.Error == nil || resp.Error.Code != ErrMethodNotFound {
		t.Fatalf("期望 -32601, 得到 %+v", resp.Error)
	}
}

func TestHandleInitialize(t *testing.T) {
	resp := NewServer(fakeProvider{}).Handle(context.Background(), &Request{Method: "initialize"})
	raw, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(raw), "protocolVersion") {
		t.Fatalf("initialize 缺少 protocolVersion: %s", raw)
	}
}

func TestHandleToolsListAndCall(t *testing.T) {
	s := NewServer(fakeProvider{})

	list := s.Handle(context.Background(), &Request{Method: "tools/list"})
	raw, _ := json.Marshal(list.Result)
	if !strings.Contains(string(raw), "self.test.ping") {
		t.Fatalf("tools/list 未包含注册工具: %s", raw)
	}

	params, _ := json.Marshal(map[string]any{"name": "self.test.ping"})
	call := s.Handle(context.Background(), &Request{Method: "tools/call", Params: params})
	raw, _ = json.Marshal(call.Result)
	if !strings.Contains(string(raw), "pong") {
		t.Fatalf("tools/call 结果异常: %s", raw)
	}
}

func TestHandleUnknownToolMapsTo32601(t *testing.T) {
	s := NewServer(fakeProvider{})
	params, _ := json.Marshal(map[string]any{"name": "nope"})
	resp := s.Handle(context.Background(), &Request{Method: "tools/call", Params: params})
	if resp.Error == nil || resp.Error.Code != ErrMethodNotFound {
		t.Fatalf("未知工具应映射 -32601 (灵矓文档), 得到 %+v", resp.Error)
	}
}
