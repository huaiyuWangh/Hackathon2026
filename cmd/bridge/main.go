// bridge 是灵矽语音驱动 Claude Code 的 Mac 桥接程序。
// serve=灵矓设备客户端(M3)；--local 提供不依赖平台的本地调试/模拟。
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/huaiyuWangh/Hackathon2026/internal/ccrunner"
	"github.com/huaiyuWangh/Hackathon2026/internal/config"
	"github.com/huaiyuWangh/Hackathon2026/internal/linx"
	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
	"github.com/huaiyuWangh/Hackathon2026/internal/tools"
	"github.com/huaiyuWangh/Hackathon2026/internal/webui"
)

const usageText = `bridge — 灵矽语音驱动 Claude Code 的 Mac 桥接程序

用法:
  bridge serve                       启动桥接服务 (灵矓设备客户端, M3 实现)
  bridge --local tools list          本地调试: 列出全部工具
  bridge --local tools call <name> [--arg key=value]...
                                     本地调试: 绕过灵矓平台直接调用工具
  bridge --local sim                 平台模拟: stdin 逐行读灵矓外层消息
                                     {"type":"mcp","payload":{JSON-RPC}}, 走完整
                                     协议链路后将响应外层消息写回 stdout

示例:
  bridge --local tools list
  bridge --local tools call dev.claude.send --arg 'prompt=修复登录测试'
  echo '{"type":"mcp","payload":{"jsonrpc":"2.0","id":1,"method":"tools/list"}}' | bridge --local sim
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
		runner, err := ccrunner.NewTmux(cfg.TmuxSession, cfg.WorkDir)
		if err != nil {
			return err
		}
		defer runner.Close()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		reg := tools.NewRegistry(runner)
		devClient := linx.New(&cfg, reg)
		// 三通道并行: 设备接入(端侧MCP+音频) + MCP Proxy + HTTP(控制页/云端MCP)
		done := make(chan error, 3)
		go func() { done <- devClient.Run(ctx) }()
		go func() { done <- linx.NewProxyClient(cfg.McpProxyBase, cfg.APIKey, reg).Run(ctx) }()
		go func() {
			sse := mcp.NewSSEServer(reg)
			webui.Mount(sse.Mux(), devClient)
			log.Printf("控制页 http://%s/", cfg.SSEAddr)
			done <- sse.ListenAndServe(cfg.SSEAddr)
		}()
		return <-done
	case local && match(args, "tools", "list"):
		return cmdToolsList()
	case local && match(args, "tools", "call"):
		return cmdToolsCall(args[2:])
	case local && match(args, "sim"):
		return cmdSim()
	case local && match(args, "mic-test"):
		return linx.MicTest(5 * time.Second)
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
	// 仅列工具描述，用 stub 不做环境检查
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
	cfg := config.Load()
	runner, err := ccrunner.NewTmux(cfg.TmuxSession, cfg.WorkDir)
	if err != nil {
		return err
	}
	defer runner.Close()
	reg := tools.NewRegistry(runner)
	res, err := reg.Call(context.Background(), name, raw)
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	return nil
}

// cmdSim 模拟灵矽平台下行：stdin 逐行读外层消息，
// 经 Envelope → JSON-RPC → mcp.Server → tools → ccrunner 完整链路后回写 stdout。
// 与 M3 的差别仅在传输层：M3 把 stdin/stdout 换成设备 WebSocket。
func cmdSim() error {
	cfg := config.Load()
	runner, err := ccrunner.NewTmux(cfg.TmuxSession, cfg.WorkDir)
	if err != nil {
		return err
	}
	defer runner.Close()
	router := linx.NewRouter(tools.NewRegistry(runner))

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if out := router.HandleEnvelope(line); out != nil {
			fmt.Println(string(out))
		}
	}
	return sc.Err()
}
