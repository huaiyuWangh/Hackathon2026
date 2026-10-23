package linx

import (
	"context"
	"errors"

	"github.com/huaiyuWangh/Hackathon2026/internal/config"
	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

// Client 灵矓设备客户端：M3 实现 WebSocket 连接/心跳/重连，
// 收到消息交给 Router，返回值回写。当前 Run 为占位。
type Client struct {
	cfg    *config.Config
	router *Router
}

// New 创建设备客户端；p 为设备对外暴露的工具集合（端侧 MCP）。
func New(cfg *config.Config, p mcp.ToolProvider) *Client {
	return &Client{cfg: cfg, router: NewRouter(p)}
}

// Run 阻塞运行直到 ctx 取消。M0 仅占位。
func (c *Client) Run(ctx context.Context) error {
	return errors.New("linx: 设备客户端将在 M3 实现; 本地可用 bridge --local sim 模拟平台下行")
}
