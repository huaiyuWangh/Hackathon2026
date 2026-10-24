package tools

import (
	"strings"
	"testing"

	"github.com/huaiyuWangh/Hackathon2026/internal/ccrunner"
)

func TestDescriptorsShape(t *testing.T) {
	descs := NewRegistry(ccrunner.NewStub()).Descriptors()
	if len(descs) == 0 {
		t.Fatal("应注册至少一个工具")
	}
	for _, d := range descs {
		if !strings.HasPrefix(d.Name, "dev.claude.") {
			t.Errorf("工具名应符合 dev.claude.* 命名: %q", d.Name)
		}
		if d.Description == "" || d.InputSchema == nil {
			t.Errorf("描述与入参 schema 不应缺失: %+v", d)
		}
	}
}
