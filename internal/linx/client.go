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

// wireMsg 一条待写消息（文本=JSON-RPC/信封，二进制=Opus 音频帧）。
type wireMsg struct {
	mt   int
	data []byte
}

// Client 灵矓设备客户端：连接 → header 鉴权 → hello 握手 → 收发循环。
// 可选音频（BRIDGE_AUDIO=1）：Mac 麦克风/扬声器即语音入口，会话锚定本设备，
// 端侧 MCP 工具随会话可用。无应用层心跳（依赖标准 WebSocket ping/pong），
// 断线指数退避重连；会话恢复靠异步工具对（重连后 dev.claude.status）。
type Client struct {
	cfg    *config.Config
	router *Router
	audio  audioDevice

	upMu sync.Mutex
	upFn func(frame []byte) // 当前连接的音频上行函数（重连时切换）

	lastAudioAt time.Time // 下行音频帧到达时刻（间隔探针）
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

	if c.cfg.Audio {
		a, err := newPortAudio()
		if err != nil {
			return err
		}
		c.audio = a
		defer a.Stop()
		if err := a.Start(c.uplink); err != nil {
			return err
		}
		log.Printf("音频已启动: 对着 Mac 说话即可（realtime 模式，服务端 VAD）")
	}

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

// SetMic 开/关麦克风上行（控制页用），返回切换后状态。
func (c *Client) SetMic(on bool) (bool, error) {
	if c.audio == nil {
		return false, errors.New("音频未启用 (BRIDGE_AUDIO=1 才能控制麦克风)")
	}
	c.audio.SetMuted(!on)
	return c.audio.Muted() == false, nil
}

// MicOn 麦克风是否开启。
func (c *Client) MicOn() bool {
	return c.audio != nil && !c.audio.Muted()
}

// uplink 音频回调：把 Opus 帧写入当前连接（无连接时丢弃）。
func (c *Client) uplink(frame []byte) {
	c.upMu.Lock()
	fn := c.upFn
	c.upMu.Unlock()
	if fn != nil {
		fn(frame)
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
	audioParams := map[string]any{}
	if c.audio != nil {
		// 小智协议惯例：format 为字符串；参数错误服务端会直接不回 hello
		audioParams = map[string]any{
			"format":         "opus",
			"sample_rate":    audioRate,
			"channels":       audioChannels,
			"frame_duration": upFrameMs,
		}
	}
	h := Hello{
		Type:        "hello",
		Version:     1,
		Features:    Features{MCP: true},
		Transport:   "websocket",
		AudioParams: audioParams,
	}
	if err := writeJSON(conn, h); err != nil {
		return fmt.Errorf("发送 hello 失败: %w", err)
	}
	conn.SetReadDeadline(time.Now().Add(helloTimeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("等待 hello 应答失败: %w", err)
	}
	if c.cfg.Debug {
		log.Printf("<< %s", msg)
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

	// 音频设备：进入 realtime 监听（服务端 VAD，检测到人声自动打断 TTS）
	if c.audio != nil {
		if err := writeJSON(conn, map[string]any{
			"type": "listen", "state": "start", "mode": "realtime",
		}); err != nil {
			return fmt.Errorf("发送 listen start 失败: %w", err)
		}
	}
	return nil
}

// loop 收发循环：当前协程读消息，writer 协程统一写出（gorilla 单写者约束）。
func (c *Client) loop(ctx context.Context, conn *websocket.Conn) error {
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readTimeout))
	})
	conn.SetReadDeadline(time.Now().Add(readTimeout))

	writes := make(chan wireMsg, 256)
	done := make(chan struct{})

	// 音频上行切换到当前连接
	c.setUplink(func(frame []byte) {
		select {
		case writes <- wireMsg{mt: websocket.BinaryMessage, data: frame}:
		default: // 写队列拥堵时丢帧（实时音频宁可丢不可等）
		}
	})
	defer c.setUplink(nil)

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
			case m := <-writes:
				if err := c.writeRaw(conn, m.mt, m.data); err != nil {
					conn.Close()
					return
				}
			}
		}
	}()

	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			close(done)
			wg.Wait()
			return err
		}
		switch {
		case mt == websocket.BinaryMessage:
			// 平台 TTS 的 Opus 音频帧；间隔偶发过大是断续的直接原因
			now := time.Now()
			if !c.lastAudioAt.IsZero() && now.Sub(c.lastAudioAt) > 150*time.Millisecond {
				log.Printf("audio: 帧到达间隔 %dms（节奏异常, 断续线索）", now.Sub(c.lastAudioAt).Milliseconds())
			}
			c.lastAudioAt = now
			if c.audio != nil {
				c.audio.PlayOpus(msg)
			}
		case mt == websocket.TextMessage:
			if c.cfg.Debug {
				log.Printf("<< %s", msg)
			}
			c.handleServerMessage(msg)
			if out := c.router.HandleEnvelope(msg); out != nil {
				select {
				case writes <- wireMsg{mt: websocket.TextMessage, data: out}:
				case <-done: // 连接已断，丢弃响应
				}
			}
		}
	}
}

// handleServerMessage 处理协议层状态消息（TTS 生命周期统计/打断清缓冲）。
func (c *Client) handleServerMessage(msg []byte) {
	var peek struct {
		Type    string `json:"type"`
		State   string `json:"state"`
		Aborted bool   `json:"is_aborted"`
	}
	if json.Unmarshal(msg, &peek) != nil || c.audio == nil {
		return
	}
	switch {
	case peek.Type == "tts" && peek.State == "start":
		c.audio.ResetStats()
		c.lastAudioAt = time.Time{}
		c.audio.SetSpeaking(true) // 回声门控：播放中抑制上行
	case peek.Type == "tts" && peek.State == "stop":
		c.audio.SetSpeaking(false) // 停播后 400ms 尾音抑制再放行
		if peek.Aborted {
			c.audio.ClearPlayback()
		}
		log.Printf("%s (本句被打断=%v)", c.audio.ReportAndReset(), peek.Aborted)
	}
}

func (c *Client) setUplink(fn func([]byte)) {
	c.upMu.Lock()
	c.upFn = fn
	c.upMu.Unlock()
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
