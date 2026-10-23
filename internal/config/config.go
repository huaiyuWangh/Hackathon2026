// Package config 提供 bridge 的运行配置，全部支持环境变量覆盖。
package config

import "os"

// Config 是桥接程序的运行配置。
// M0 只定义字段与默认值；M3 接入灵矽时补齐设备鉴权相关的使用。
type Config struct {
	// LinxWSURL 灵矽设备接入 WebSocket 地址。
	LinxWSURL string
	// DeviceID 设备标识，hello 消息中上报。
	DeviceID string
	// APIKey 灵矽控制台创建的 API KEY（serve 模式鉴权用）。
	APIKey string
	// TmuxSession 承载交互式 Claude Code 的 tmux 会话名（唯一"现场"）。
	TmuxSession string
	// WorkDir Claude Code 的工作目录。
	WorkDir string
}

// Load 从环境变量读取配置，缺省用默认值。
func Load() Config {
	return Config{
		LinxWSURL:   getenv("BRIDGE_LINX_WS_URL", "ws://xrobo-io.qiniuapi.com/v1/ws/"),
		DeviceID:    getenv("BRIDGE_DEVICE_ID", "bridge-mac-001"),
		APIKey:      getenv("BRIDGE_API_KEY", ""),
		TmuxSession: getenv("BRIDGE_TMUX_SESSION", "ccline"),
		WorkDir:     getenv("BRIDGE_WORK_DIR", "."),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
