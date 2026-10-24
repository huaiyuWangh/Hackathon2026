package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/huaiyuWangh/Hackathon2026/internal/ccrunner"
	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

type fakeRunner struct {
	sendErr    error
	sendID     string
	lastPrompt string
	status     ccrunner.TaskStatus
	statErr    error
	intErr     error
}

func (f *fakeRunner) Ensure(ctx context.Context) error { return nil }
func (f *fakeRunner) Send(p string) (string, error) {
	f.lastPrompt = p
	if f.sendErr != nil {
		return "", f.sendErr
	}
	return f.sendID, nil
}
func (f *fakeRunner) Status(id string) (ccrunner.TaskStatus, error) { return f.status, f.statErr }
func (f *fakeRunner) Interrupt() error                              { return f.intErr }
func (f *fakeRunner) Subscribe() ccrunner.View                      { return ccrunner.View{} }

func mustCall(t *testing.T, reg *Registry, name string, args string) *mcp.CallResult {
	t.Helper()
	res, err := reg.Call(context.Background(), name, json.RawMessage(args))
	if err != nil {
		t.Fatalf("Call(%s) 意外错误: %v", name, err)
	}
	return res
}

func TestUnknownToolWrapsSentinel(t *testing.T) {
	reg := NewRegistry(&fakeRunner{})
	_, err := reg.Call(context.Background(), "dev.nope", nil)
	if !errors.Is(err, mcp.ErrUnknownTool) {
		t.Fatalf("未知工具应包装 mcp.ErrUnknownTool: %v", err)
	}
}

func TestClaudeSendValidation(t *testing.T) {
	reg := NewRegistry(&fakeRunner{})
	res := mustCall(t, reg, "dev.claude.send", `{"prompt":""}`)
	if !res.IsError {
		t.Fatal("空 prompt 应返回业务错误")
	}
	res = mustCall(t, reg, "dev.claude.send", `{}`)
	if !res.IsError {
		t.Fatal("缺 prompt 应返回业务错误")
	}
}

func TestClaudeSendOK(t *testing.T) {
	fr := &fakeRunner{sendID: "t9"}
	reg := NewRegistry(fr)
	res := mustCall(t, reg, "dev.claude.send", `{"prompt":"修复测试"}`)
	if res.IsError || !strings.Contains(res.Content[0].Text, "task=t9") {
		t.Fatalf("send 成功结果异常: %+v", res)
	}
	if fr.lastPrompt != "修复测试" {
		t.Fatalf("prompt 未透传: %q", fr.lastPrompt)
	}
}

func TestClaudeSendRunnerError(t *testing.T) {
	reg := NewRegistry(&fakeRunner{sendErr: errors.New("boom")})
	if _, err := reg.Call(context.Background(), "dev.claude.send", json.RawMessage(`{"prompt":"x"}`)); err == nil {
		t.Fatal("runner 错误应向上传递")
	}
}

func TestClaudeStatusAndInterrupt(t *testing.T) {
	reg := NewRegistry(&fakeRunner{status: ccrunner.TaskStatus{ID: "t1", State: ccrunner.TaskDone, Summary: "ok"}})
	res := mustCall(t, reg, "dev.claude.status", `{}`)
	if res.IsError || !strings.Contains(res.Content[0].Text, "done") {
		t.Fatalf("status 结果异常: %+v", res)
	}
	res = mustCall(t, reg, "dev.claude.interrupt", `{}`)
	if res.IsError {
		t.Fatalf("interrupt 结果异常: %+v", res)
	}
}
