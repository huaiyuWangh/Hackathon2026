// Package config 提供 bridge 的运行配置，全部支持环境变量覆盖；
// 存在 .env（参考 .env.example）时作为兜底装载，已设置的环境变量优先。
package config

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Config 是桥接程序的运行配置。
type Config struct {
	// LinxWSURL 灵矽设备接入 WebSocket 地址（生产用 wss）。
	LinxWSURL string
	// DeviceID 即握手 header Device-Id：平台用于验证设备绑定，应为物理网卡 MAC。
	DeviceID string
	// ClientID 即握手 header Client-Id：软件生成，持久化以便断线重连后恢复会话。
	ClientID string
	// APIKey 灵矽控制台凭证，Authorization: Bearer 使用。
	APIKey string
	// TmuxSession 承载交互式 Claude Code 的 tmux 会话名（唯一"现场"）。
	TmuxSession string
	// WorkDir Claude Code 的工作目录（同时存放 .bridge 运行时状态）。
	WorkDir string
	// McpProxyBase 灵矓平台 API 基地址（MCP Proxy 用）。
	McpProxyBase string
	// SSEAddr 云端 MCP 的 HTTP+SSE 监听地址（配合 cloudflared 隧道暴露）。
	SSEAddr string
	// ViewerAddr 网页查看器监听地址（局域网可访问，只读直播）。
	ViewerAddr string
	// Audio 启用设备音频（Mac 麦克风/扬声器作为语音入口，端侧 MCP 会话关键）。
	Audio bool
	// Debug 输出收发的原始消息（联调排障用）。
	Debug bool
}

// Load 从环境变量/.env 读取配置并补默认值。
func Load() Config {
	loadDotEnv(".env")

	workDir := getenv("BRIDGE_WORK_DIR", ".")
	return Config{
		LinxWSURL:    getenv("BRIDGE_LINX_WS_URL", "wss://xrobo-io.qiniuapi.com/v1/ws/"),
		DeviceID:     getenv("BRIDGE_DEVICE_ID", detectMAC()),
		ClientID:     clientID(filepath.Join(workDir, ".bridge")),
		APIKey:       getenv("BRIDGE_API_KEY", ""),
		TmuxSession:  getenv("BRIDGE_TMUX_SESSION", "ccline"),
		WorkDir:      workDir,
		McpProxyBase: getenv("BRIDGE_MCP_PROXY_BASE", "https://xrobo.qiniu.com"),
		SSEAddr:      getenv("BRIDGE_SSE_ADDR", "127.0.0.1:8787"),
		ViewerAddr:   getenv("BRIDGE_VIEWER_ADDR", "0.0.0.0:8788"),
		Audio:        getenv("BRIDGE_AUDIO", "") != "",
		Debug:        getenv("BRIDGE_DEBUG", "") != "",
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadDotEnv 极简 .env 装载：KEY=VALUE 行，# 注释；不覆盖已有环境变量。
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// detectMAC 返回 en0 的 MAC 地址（平台用它校验设备绑定）；取不到时回退占位值。
func detectMAC() string {
	out, err := exec.Command("ifconfig", "en0").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "ether" {
				return fields[1]
			}
		}
	}
	return "bridge-mac-001"
}

// clientID 读取或生成并持久化 Client-Id：固定 Client-Id 重连，
// 平台侧更可能延续会话（同 Device-Id 重复连接的互踢行为待联调确认）。
func clientID(stateDir string) string {
	p := filepath.Join(stateDir, "client-id")
	if b, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return string(bytes.TrimSpace(b))
	}
	id := randomID()
	_ = os.MkdirAll(stateDir, 0o755)
	_ = os.WriteFile(p, []byte(id), 0o644)
	return id
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
