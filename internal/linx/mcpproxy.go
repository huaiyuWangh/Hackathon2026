package linx

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

// ProxyClient 通过灵矽 MCP Proxy（WebSocket 代理）把本机 MCP 服务暴露给平台，
// 无需公网地址——连接方向是本机主动出站。代理侧扮演 MCP 客户端：
// 连接后主动下发 initialize / tools/list / tools/call（裸 JSON-RPC，无灵矽外层信封），
// 与设备端 MCP 的差别仅在传输层与消息封装。
type ProxyClient struct {
	base   string // 平台 API 基地址，如 https://xrobo.qiniu.com
	apiKey string
	server *mcp.Server
}

// NewProxyClient 创建代理客户端；p 为本机暴露的工具集合。
func NewProxyClient(base, apiKey string, p mcp.ToolProvider) *ProxyClient {
	return &ProxyClient{base: base, apiKey: apiKey, server: mcp.NewServer(p)}
}

// Run 阻塞运行直到 ctx 取消；断线指数退避重连。
func (c *ProxyClient) Run(ctx context.Context) error {
	if c.apiKey == "" {
		return fmt.Errorf("mcp-proxy: 缺少凭证, 请在 .env 设置 BRIDGE_API_KEY")
	}
	backoff := initialBackoff
	for {
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("mcp-proxy 连接断开: %v; %s 后重连", err, backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (c *ProxyClient) runOnce(ctx context.Context) error {
	wss, err := c.token(ctx)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wss, nil)
	if err != nil {
		return fmt.Errorf("代理连接失败: %w", err)
	}
	defer conn.Close()
	log.Printf("mcp-proxy 已连接 (%s)", c.base)

	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if mt != websocket.TextMessage {
			continue
		}
		var req mcp.Request
		if err := json.Unmarshal(msg, &req); err != nil || req.Method == "" {
			log.Printf("mcp-proxy: 忽略无法解析的消息: %.200s", msg)
			continue
		}
		if req.Method == "initialize" {
			log.Printf("mcp-proxy: 收到 initialize")
		}
		resp := c.server.Handle(ctx, &req)
		if len(req.ID) == 0 {
			continue // 通知不回包
		}
		out, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
			return err
		}
	}
}

// token 获取代理连接地址（30 天有效，每次连接前现取）。
func (c *ProxyClient) token(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/mcpproxy/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Code int `json:"code"`
		Data struct {
			Wss string `json:"wss_endpoint"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Code != 0 || out.Data.Wss == "" {
		return "", fmt.Errorf("获取 mcpproxy token 失败: code=%d err=%v", out.Code, err)
	}
	return out.Data.Wss, nil
}
