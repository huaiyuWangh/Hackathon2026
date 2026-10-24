package ccrunner

import (
	"context"
	"strings"
	"testing"
)

func TestStubAlwaysErrors(t *testing.T) {
	s := NewStub()
	for name, fn := range map[string]func() error{
		"Ensure":    func() error { return s.Ensure(context.Background()) },
		"Send":      func() error { _, err := s.Send("hi"); return err },
		"Status":    func() error { _, err := s.Status(""); return err },
		"Interrupt": s.Interrupt,
	} {
		if err := fn(); err == nil || !strings.Contains(err.Error(), "占位") {
			t.Errorf("%s 应返回占位错误: %v", name, err)
		}
	}
	if v := s.Subscribe(); v.Ch != nil || v.Cancel != nil {
		t.Errorf("占位订阅应为零值: %+v", v)
	}
}
