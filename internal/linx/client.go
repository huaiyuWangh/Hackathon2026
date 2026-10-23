package linx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/huaiyuWangh/Hackathon2026/internal/config"
	"github.com/huaiyuWangh/Hackathon2026/internal/mcp"
)

const (
	protocolVersion = "1" // header Protocol-Version，与 hello.version 一致
	helloTimeout    = 10 * time.Second
	pingPeriod      = 30 * time.Second
	readTimeout     = 90 * time.Second // ≥ 2×ping；收到 pong 续期
	writeTimeout    = 10 * time.Second
	dialTimeout     = 10 * time.Second
	initialBackoff  = time.Second
	maxBackoff      = 30 * time.Second
)

// Client 灵矽设备客户端：连接 → header 鉴权 → hello 握手 → 收发循环。
// 无应用层心跳（官方依赖标准 WebSocket ping/pong），断线指数退避重连；
// 会话恢复靠异步工具对：重连后智能体调 dev.claude.status 拿回现场。
type Client struct {
	cfg    *config.Config
	router *Router
}

// New 创建设备客户端；p 为设备对外暴露的工具集合（端侧 MCP）。
func New(cfg *config.Config, p mcp.ToolProvider) *Client {
	return &Client{cfg: cfg, router: NewRouter(p)}
}

// Run 阻塞运行直到 ctx 取消；断线自动重连。
func (c *Client) Run(ctx context.Context) error {
	if c.cfg.APIKey == "" {
		return errors.New("linx: 缺少凭证, 请在 .env 设置 BRIDGE_API_KEY (参考 .env.example)")
	}
	log.SetPrefix("bridge-serve ")
	log.SetFlags(log.Ltime)

	backoff := initialBackoff
	for {
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return nil // 主动退出
		}
		log.Printf("连接断开: %v; %s 后重连", err, backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	header := http.Header{
		"Authorization":    {"Bearer " + c.cfg.APIKey},
		"Protocol-Version": {protocolVersion},
		"Device-Id":        {c.cfg.DeviceID},
		"Client-Id":        {c.cfg.ClientID},
	}
	dialer := websocket.Dialer{HandshakeTimeout: dialTimeout}
	conn, resp, err := dialer.DialContext(ctx, c.cfg.LinxWSURL, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("连接失败: %w (http %d)", err, resp.StatusCode)
		}
		return fmt.Errorf("连接失败: %w", err)
	}
	defer conn.Close()
	log.Printf("已连接 %s (device %s client %s)", c.cfg.LinxWSURL, c.cfg.DeviceID, c.cfg.ClientID)

	if err := c.handshake(conn); err != nil {
		return err
	}
	return c.loop(ctx, conn)
}

// handshake 发送 hello 并等待服务端 hello 应答。
func (c *Client) handshake(conn *websocket.Conn) error {
	h := Hello{
		Type:        "hello",
		Version:     1,
		Features:    Features{MCP: true},
		Transport:   "websocket",
		AudioParams: map[string]any{},
	}
	if err := writeJSON(conn, h); err != nil {
		return fmt.Errorf("发送 hello 失败: %w", err)
	}
	conn.SetReadDeadline(time.Now().Add(helloTimeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("等待 hello 应答失败: %w", err)
	}
	var sh ServerHello
	if err := json.Unmarshal(msg, &sh); err != nil || sh.Type != "hello" {
		return fmt.Errorf("hello 应答异常: %.200s", msg)
	}
	sid := sh.SessionID
	if sid == "" {
		sid = "(无 session_id)"
	}
	log.Printf("握手完成 session=%s", sid)
	return nil
}

// loop 收发循环：当前协程读消息交给 Router，单独协程负责写（响应+ping）。
// gorilla/websocket 只允许一个并发写者，故全部写集中在 writer 协程。
func (c *Client) loop(ctx context.Context, conn *websocket.Conn) error {
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readTimeout))
	})
	conn.SetReadDeadline(time.Now().Add(readTimeout))

	writes := make(chan []byte, 16)
	done := make(chan struct{}) // 读循环退出信号，用于收尾 writer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case <-done:
				conn.Close()
				return
			case <-ticker.C:
				if err := c.writeRaw(conn, websocket.PingMessage, nil); err != nil {
					conn.Close()
					return
				}
			case data := <-writes:
				if err := c.writeRaw(conn, websocket.TextMessage, data); err != nil {
					conn.Close()
					return
				}
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			close(done)
			wg.Wait()
			return err
		}
		if c.cfg.Debug {
			log.Printf("<< %s", msg)
		}
		if out := c.router.HandleEnvelope(msg); out != nil {
			select {
			case writes <- out:
			case <-done: // 连接已断，丢弃响应
			}
		}
	}
}

func (c *Client) writeRaw(conn *websocket.Conn, mt int, data []byte) error {
	if c.cfg.Debug && mt == websocket.TextMessage {
		log.Printf(">> %s", data)
	}
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteMessage(mt, data)
}

func writeJSON(conn *websocket.Conn, v any) error {
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteJSON(v)
}
