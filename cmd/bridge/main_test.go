package main

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	args := []string{"tools", "list", "extra"}
	if !match(args, "tools", "list") {
		t.Fatal("前缀应匹配")
	}
	if match(args, "tools", "call") {
		t.Fatal("不同子命令不应匹配")
	}
	if match([]string{"tools"}, "tools", "list") {
		t.Fatal("长度不足不应匹配")
	}
}

func TestRunUnknownArgs(t *testing.T) {
	if err := run([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "无法识别") {
		t.Fatalf("未知参数应报错: %v", err)
	}
}

// 本地调试命令不依赖 tmux/claude（tools list 用 stub），可安全执行。
func TestRunLocalToolsList(t *testing.T) {
	if err := run([]string{"--local", "tools", "list"}); err != nil {
		t.Fatalf("本地列工具应成功: %v", err)
	}
}

func TestCmdToolsCallArgValidation(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, "缺少工具名"},
		{[]string{"t", "plain"}, "只支持 --arg"},
		{[]string{"t", "--arg"}, "缺少值"},
		{[]string{"t", "--arg", "novalue"}, "key=value"},
	}
	for _, c := range cases {
		err := cmdToolsCall(c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("cmdToolsCall(%v) 应含 %q: %v", c.args, c.want, err)
		}
	}
}
