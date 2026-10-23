// bridge 是灵矽语音驱动 Claude Code 的 Mac 桥接程序。
// M0 阶段：装配骨架 + --local 本地调试 CLI；serve 模式（灵矽设备客户端）M3 实现。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/huaiyuWangh/Hackathon2026/internal/ccrunner"
	"github.com/huaiyuWangh/Hackathon2026/internal/config"
	"github.com/huaiyuWangh/Hackathon2026/internal/linx"
	"github.com/huaiyuWangh/Hackathon2026/internal/tools"
)

const usageText = `bridge — 灵矽语音驱动 Claude Code 的 Mac 桥接程序 (M0 骨架)

用法:
  bridge serve                       启动桥接服务 (灵矽设备客户端, M3 实现)
  bridge --local tools list          本地调试: 列出全部工具
  bridge --local tools call <name> [--arg key=value]...
                                     本地调试: 绕过灵矽平台直接调用工具

示例:
  bridge --local tools list
  bridge --local tools call dev.claude.send --arg 'prompt=修复登录测试'
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}

	local := false
	var args []string
	for _, a := range argv {
		if a == "--local" {
			local = true
			continue
		}
		args = append(args, a)
	}

	switch {
	case match(args, "serve"):
		cfg := config.Load()
		return linx.New(&cfg).Run(context.Background())
	case local && match(args, "tools", "list"):
		return cmdToolsList()
	case local && match(args, "tools", "call"):
		return cmdToolsCall(args[2:])
	default:
		return fmt.Errorf("无法识别的参数 %v\n\n%s", argv, usageText)
	}
}

// match 判断 args 的前缀是否等于 want。
func match(args []string, want ...string) bool {
	if len(args) < len(want) {
		return false
	}
	for i, w := range want {
		if args[i] != w {
			return false
		}
	}
	return true
}

func cmdToolsList() error {
	reg := tools.NewRegistry(ccrunner.NewStub())
	fmt.Printf("%-22s %s\n", "TOOL", "DESCRIPTION")
	for _, d := range reg.Descriptors() {
		fmt.Printf("%-22s %s\n", d.Name, d.Description)
	}
	return nil
}

func cmdToolsCall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("缺少工具名, 例: bridge --local tools call dev.claude.send --arg prompt=你好")
	}
	name := args[0]
	callArgs := map[string]any{}
	for i := 1; i < len(args); i++ {
		if args[i] != "--arg" {
			return fmt.Errorf("只支持 --arg key=value, 得到 %q", args[i])
		}
		i++
		if i >= len(args) {
			return fmt.Errorf("--arg 缺少值")
		}
		kv := strings.SplitN(args[i], "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("--arg 需要 key=value 形式, 得到 %q", args[i])
		}
		callArgs[kv[0]] = kv[1]
	}
	raw, err := json.Marshal(callArgs)
	if err != nil {
		return err
	}
	reg := tools.NewRegistry(ccrunner.NewStub())
	res, err := reg.Call(context.Background(), name, raw)
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	return nil
}
