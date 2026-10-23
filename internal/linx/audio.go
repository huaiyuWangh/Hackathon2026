package linx

import (
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
)

// 设备音频：麦克风采集 → Opus 编码上行；平台 TTS 的 Opus 帧 → 解码 → 扬声器。
// realtime 模式下持续上行，VAD 与人声打断由服务端负责。

const (
	audioRate     = 16000 // 上行采样率（协议要求 16kHz 单声道）
	audioChannels = 1
	upFrameMs     = 60
	upSamples     = audioRate * upFrameMs / 1000 // 960
	playChunk     = 480                          // 下行播写粒度 30ms
)

// audioDevice 是设备音频能力的抽象（无音频时用 nil 关闭相关路径）。
type audioDevice interface {
	// Start 启动采集/播放；onUp 收到编码后的 Opus 帧用于二进制上行。
	Start(onUp func(frame []byte)) error
	Stop()
	// PlayOpus 解码并排队播放一帧平台 TTS。
	PlayOpus(frame []byte)
	// ClearPlayback 清空播放缓冲（tts stop is_aborted=true 时调用）。
	ClearPlayback()
	// SetMuted 静音/恢复上行（控制页开关麦克风）。
	SetMuted(muted bool)
	// Muted 当前是否静音。
	Muted() bool
	// SetSpeaking TTS 播放开始/结束（回声门控：播放中抑制上行）。
	SetSpeaking(on bool)
	// ResetStats 清零播放统计（tts start）。
	ResetStats()
	// ReportAndReset 输出并清零统计（tts stop）。
	ReportAndReset() string
}

// portAudio 基于 PortAudio + libopus 的实现。
// 播放走**回调模式**（音频线程主动拉取）：Go 协程阻塞式写入会被运行时
// 调度/GC 周期性打断（实测断续的最小公分母），回调模式彻底避开。
type portAudio struct {
	enc *opus.Encoder
	dec *opus.Decoder
	in  *portaudio.Stream
	out *portaudio.Stream

	playCh chan []int16
	quit   chan struct{}
	once   sync.Once

	firstFrameOnce sync.Once // 首帧 TTS 日志
	decErrs        atomic.Int32
	playDrops      atomic.Int32

	pmu         sync.Mutex // 播放缓冲
	playBuf     []int16
	playStarted bool

	gmu        sync.Mutex // 回声门控：TTS 播放中抑制上行（无耳机的半双工策略）
	gating     bool
	gateOpenAt time.Time

	mu    sync.Mutex
	muted bool // 默认静音，由控制页开启
}

// newPortAudio 创建音频设备（未启动，Start 时开流）。
func newPortAudio() (*portAudio, error) {
	enc, err := opus.NewEncoder(audioRate, audioChannels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("audio: opus 编码器创建失败: %w", err)
	}
	dec, err := opus.NewDecoder(audioRate, audioChannels)
	if err != nil {
		return nil, fmt.Errorf("audio: opus 解码器创建失败: %w", err)
	}
	return &portAudio{
		enc:    enc,
		dec:    dec,
		playCh: make(chan []int16, 128),
		quit:   make(chan struct{}),
		muted:  true, // 默认静音，由控制页开启
	}, nil
}

// Start 打开输入/输出流并进入持续采集。
func (a *portAudio) Start(onUp func(frame []byte)) error {
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("audio: 初始化失败: %w", err)
	}
	inBuf := make([]int16, upSamples)
	in, err := portaudio.OpenDefaultStream(audioChannels, 0, audioRate, len(inBuf), inBuf)
	if err != nil {
		return fmt.Errorf("audio: 打开麦克风失败(检查系统麦克风权限): %w", err)
	}
	// 输出用回调模式（音频线程拉取，消除 Go 调度抖动）；256 帧 ≈16ms 回调粒度
	out, err := portaudio.OpenDefaultStream(0, audioChannels, audioRate, 256, a.outCallback)
	if err != nil {
		in.Close()
		return fmt.Errorf("audio: 打开扬声器失败: %w", err)
	}
	a.in, a.out = in, out
	if err := in.Start(); err != nil {
		return fmt.Errorf("audio: 启动采集失败: %w", err)
	}
	if err := out.Start(); err != nil {
		return fmt.Errorf("audio: 启动播放失败: %w", err)
	}

	// 麦克风循环：60ms 一帧 → Opus 编码 → 上行（静音时只排水不发）
	go func() {
		payload := make([]byte, 1024)
		for {
			select {
			case <-a.quit:
				return
			default:
			}
			if err := in.Read(); err != nil {
				continue
			}
			if a.Muted() || a.echoGated() {
				continue
			}
			n, err := a.enc.Encode(inBuf, payload)
			if err != nil || n == 0 {
				continue
			}
			onUp(payload[:n])
		}
	}()

	// 供水中转：playCh → playBuf（回调线程从这里拉）
	go func() {
		const maxBuf = audioRate * 3 / 2 // 1.5s 上限，防无限积压
		for {
			select {
			case <-a.quit:
				return
			case seg := <-a.playCh:
				a.pmu.Lock()
				if len(a.playBuf) < maxBuf {
					a.playBuf = append(a.playBuf, seg...)
				}
				a.pmu.Unlock()
			}
		}
	}()
	return nil
}

// outCallback 音频线程回调：初始蓄水 300ms 后持续排空；不足补零。
func (a *portAudio) outCallback(out []int16) {
	const minSamples = audioRate * 300 / 1000 // 初始蓄水
	a.pmu.Lock()
	defer a.pmu.Unlock()
	if !a.playStarted {
		if len(a.playBuf) < minSamples {
			for i := range out {
				out[i] = 0
			}
			return
		}
		a.playStarted = true
	}
	n := copy(out, a.playBuf)
	a.playBuf = a.playBuf[n:]
	for i := n; i < len(out); i++ {
		out[i] = 0
	}
}

// Stop 停止并释放音频资源。
func (a *portAudio) Stop() {
	a.once.Do(func() { close(a.quit) })
	if a.in != nil {
		a.in.Close()
	}
	if a.out != nil {
		a.out.Close()
	}
	portaudio.Terminate()
}

// PlayOpus 解码一帧 TTS 音频并入队播放。
func (a *portAudio) PlayOpus(frame []byte) {
	a.firstFrameOnce.Do(func() { log.Printf("audio: 收到首帧 TTS 音频 (%d 字节), 开始播放", len(frame)) })
	pcm := make([]int16, audioRate) // 60ms@16k 上限，解码器按帧长填充
	n, err := a.dec.Decode(frame, pcm)
	if err != nil || n == 0 {
		a.decErrs.Add(1)
		return
	}
	select {
	case a.playCh <- pcm[:n]:
	default:
		a.playDrops.Add(1) // 播放跟不上时丢弃，避免无限积压
	}
}

// ResetStats 清零本轮播放统计。
func (a *portAudio) ResetStats() {
	a.decErrs.Store(0)
	a.playDrops.Store(0)
}

// ReportAndReset 返回并清零本轮播放统计（每句 TTS 结束时打日志用）。
func (a *portAudio) ReportAndReset() string {
	s := fmt.Sprintf("audio 统计: 解码失败=%d 入队丢弃=%d",
		a.decErrs.Load(), a.playDrops.Load())
	a.decErrs.Store(0)
	a.playDrops.Store(0)
	return s
}

// ClearPlayback 清空待播缓冲（外部队列 + 回调缓冲 + 蓄水闸门复位）。
func (a *portAudio) ClearPlayback() {
	for {
		select {
		case <-a.playCh:
		default:
			a.pmu.Lock()
			a.playBuf = a.playBuf[:0]
			a.playStarted = false
			a.pmu.Unlock()
			return
		}
	}
}

// SetMuted 静音/恢复麦克风上行。
func (a *portAudio) SetMuted(muted bool) {
	a.mu.Lock()
	a.muted = muted
	a.mu.Unlock()
}

// Muted 当前是否静音。
func (a *portAudio) Muted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.muted
}

// SetSpeaking 通知 TTS 播放开始/结束：播放中与结束后 400ms 内抑制上行，
// 防止扬声器声音被自家麦克风拾回造成"自问自答"的回声循环（实测踩坑）。
func (a *portAudio) SetSpeaking(on bool) {
	a.gmu.Lock()
	defer a.gmu.Unlock()
	if on {
		a.gating = true
	} else {
		a.gating = false
		a.gateOpenAt = time.Now().Add(400 * time.Millisecond)
	}
}

// echoGated 当前是否处于回声抑制窗口。
func (a *portAudio) echoGated() bool {
	a.gmu.Lock()
	defer a.gmu.Unlock()
	return a.gating || time.Now().Before(a.gateOpenAt)
}

// PlayTest 本地播放自检（不经平台）：阶段1按 60ms 实时节奏喂 50 帧，
// 阶段2一次性突发 50 帧。全程应听到连续不断的 440Hz 音；有断续则本地播放有问题。
func PlayTest() error {
	a, err := newPortAudio()
	if err != nil {
		return err
	}
	if err := a.Start(func([]byte) {}); err != nil {
		return err
	}
	defer a.Stop()
	enc, err := opus.NewEncoder(audioRate, audioChannels, opus.AppVoIP)
	if err != nil {
		return err
	}
	feed := func(idx int) {
		pcm := make([]int16, upSamples)
		for i := range pcm {
			pcm[i] = int16(12000 * math.Sin(2*math.Pi*440*float64(idx*upSamples+i)/audioRate))
		}
		data := make([]byte, 1024)
		n, err := enc.Encode(pcm, data)
		if err == nil && n > 0 {
			a.PlayOpus(data[:n])
		}
	}
	fmt.Println("阶段1: 实时节奏喂帧 3s —— 应连续")
	for i := 0; i < 50; i++ {
		feed(i)
		time.Sleep(upFrameMs * time.Millisecond)
	}
	fmt.Println("阶段2: 突发喂帧 3s —— 抖动缓冲应吸收, 仍应连续")
	for i := 50; i < 100; i++ {
		feed(i)
	}
	fmt.Println("喂帧完毕, 等待播完…")
	time.Sleep(5 * time.Second)
	fmt.Println("完成: 两段都连续 → 本地播放正常, 断续来自服务端节奏或回声打断")
	return nil
}

// MicTest 实测默认输入设备录音 dur，打印峰值（排障：权限被拒时 macOS 给全零数据）。
func MicTest(d time.Duration) error {
	if err := portaudio.Initialize(); err != nil {
		return err
	}
	defer portaudio.Terminate()
	buf := make([]int16, upSamples)
	in, err := portaudio.OpenDefaultStream(audioChannels, 0, audioRate, len(buf), buf)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := in.Start(); err != nil {
		return err
	}
	deadline := time.Now().Add(d)
	var peak int16
	frames := 0
	for time.Now().Before(deadline) {
		if err := in.Read(); err != nil {
			continue
		}
		frames++
		for _, s := range buf {
			v := s
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
	}
	fmt.Printf("录音 %.1fs: %d 帧, 峰值 %d/32767 (%.1f%%)\n", d.Seconds(), frames, peak, float64(peak)/32767*100)
	if peak < 300 {
		fmt.Println("⚠️ 几乎无声: 麦克风权限未授予（系统静默给全零）或默认输入设备不对")
		fmt.Println("   排查: 系统设置 → 隐私与安全性 → 麦克风 → 勾选运行 bridge 的终端 App 后重启服务")
		return errors.New("mic: silent")
	}
	fmt.Println("✅ 麦克风正常收音")
	return nil
}
