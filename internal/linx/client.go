package linx

import (
	"context"
	"errors"

	"github.com/huaiyuWangh/Hackathon2026/internal/config"
)

// Client 灵矽设备客户端骨架：M3 实现 WebSocket 连接/心跳/重连，
// 并把 type=mcp 的消息交给 mcp.Server 处理、音频帧交给 audio 处理（可选）。
type Client struct {
	cfg *config.Config
}

// New 创建设备客户端。
func New(cfg *config.Config) *Client { return &Client{cfg: cfg} }

// Run 阻塞运行直到 ctx 取消。M0 仅占位。
func (c *Client) Run(ctx context.Context) error {
	return errors.New("linx: 设备客户端将在 M3 实现, 当前请用 bridge --local 调试")
}
