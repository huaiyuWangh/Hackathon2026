package ccrunner

import (
	"context"
	"errors"
)

// NewStub 返回占位 Runner：无 tmux 环境也能列工具/装配骨架，真实驱动用 NewTmux。
func NewStub() Runner { return stub{} }

type stub struct{}

func (stub) Ensure(ctx context.Context) error         { return errNoTmux() }
func (stub) Send(prompt string) (string, error)       { return "", errNoTmux() }
func (stub) Status(taskID string) (TaskStatus, error) { return TaskStatus{}, errNoTmux() }
func (stub) Interrupt() error                         { return errNoTmux() }
func (stub) Subscribe() View                          { return View{} }

func errNoTmux() error { return errors.New("ccrunner: 占位实现, 请用 NewTmux") }
