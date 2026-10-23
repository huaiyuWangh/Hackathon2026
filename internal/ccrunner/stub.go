package ccrunner

import (
	"context"
	"errors"
)

// NewStub 返回占位 Runner：M0 骨架可编译、--local CLI 可跑通，
// 真实 tmux 驱动（M1）完成后替换。
func NewStub() Runner { return stub{} }

type stub struct{}

func (stub) Ensure(ctx context.Context) error         { return errM1() }
func (stub) Send(prompt string) (string, error)       { return "", errM1() }
func (stub) Status(taskID string) (TaskStatus, error) { return TaskStatus{}, errM1() }
func (stub) Interrupt() error                         { return errM1() }
func (stub) Output() <-chan Chunk                     { return nil }

func errM1() error { return errors.New("ccrunner: M1 尚未实现 (tmux 驱动)") }
