package linx

import (
	"errors"
	"fmt"
	"sync"
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
}

// portAudio 基于 PortAudio + libopus 的实现。
type portAudio struct {
	enc *opus.Encoder
	dec *opus.Decoder
	in  *portaudio.Stream
	out *portaudio.Stream

	playCh chan []int16
	quit   chan struct{}
	once   sync.Once

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
		playCh: make(chan []int16, 128), // ~2s 缓冲
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
	outBuf := make([]int16, playChunk)
	out, err := portaudio.OpenDefaultStream(0, audioChannels, audioRate, playChunk, outBuf)
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
			if a.Muted() {
				continue
			}
			n, err := a.enc.Encode(inBuf, payload)
			if err != nil || n == 0 {
				continue
			}
			onUp(payload[:n])
		}
	}()

	// 播放循环：playCh 累积 → 30ms 粒度写入扬声器
	go func() {
		var acc []int16
		for {
			select {
			case <-a.quit:
				return
			case seg := <-a.playCh:
				acc = append(acc, seg...)
				for len(acc) >= playChunk {
					copy(outBuf, acc[:playChunk])
					acc = acc[playChunk:]
					if err := out.Write(); err != nil {
						acc = nil
						return
					}
				}
			}
		}
	}()
	return nil
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
	pcm := make([]int16, audioRate) // 60ms@16k 上限，解码器按帧长填充
	n, err := a.dec.Decode(frame, pcm)
	if err != nil || n == 0 {
		return
	}
	select {
	case a.playCh <- pcm[:n]:
	default: // 播放跟不上时丢弃旧帧，避免无限积压
	}
}

// ClearPlayback 清空待播缓冲。
func (a *portAudio) ClearPlayback() {
	for {
		select {
		case <-a.playCh:
		default:
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
