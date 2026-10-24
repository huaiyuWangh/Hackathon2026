package linx

import (
	"math"
	"strings"
	"testing"
	"time"
)

// 音频设备纯逻辑测试：不打开真实音频流（newPortAudio 只建编解码器），
// 覆盖回声门控、静音开关、播放统计与缓冲清理。

func TestPortAudioMutedDefaultAndToggle(t *testing.T) {
	a, err := newPortAudio()
	if err != nil {
		t.Skipf("opus 编解码器不可用, 跳过: %v", err)
	}
	if !a.Muted() {
		t.Fatal("默认应静音（由控制页开启）")
	}
	a.SetMuted(false)
	if a.Muted() {
		t.Fatal("SetMuted(false) 后不应静音")
	}
}

func TestPortAudioEchoGate(t *testing.T) {
	a, _ := newPortAudio()
	if a.echoGated() {
		t.Fatal("未开播不应处于门控窗口")
	}
	a.SetSpeaking(true)
	if !a.echoGated() {
		t.Fatal("TTS 播放中应抑制上行")
	}
	a.SetSpeaking(false)
	if !a.echoGated() {
		t.Fatal("停播后 400ms 尾音窗口内仍应抑制")
	}
	time.Sleep(450 * time.Millisecond)
	if a.echoGated() {
		t.Fatal("尾音窗口结束后应放行上行")
	}
}

func TestPortAudioPlayOpusStats(t *testing.T) {
	a, _ := newPortAudio()

	// 非法帧：解码失败计入统计
	a.PlayOpus([]byte("not-opus"))
	if s := a.ReportAndReset(); !strings.Contains(s, "解码失败=1") {
		t.Fatalf("坏帧应计入解码失败: %s", s)
	}
	// ReportAndReset 已清零
	if s := a.ReportAndReset(); !strings.Contains(s, "解码失败=0") {
		t.Fatalf("统计应已清零: %s", s)
	}

	// 合法 Opus 帧：编码 60ms 正弦再喂给播放队列
	pcm := make([]int16, upSamples)
	for i := range pcm {
		pcm[i] = int16(8000 * math.Sin(2*math.Pi*440*float64(i)/audioRate))
	}
	payload := make([]byte, 1024)
	n, err := a.enc.Encode(pcm, payload)
	if err != nil || n == 0 {
		t.Fatalf("编码失败: %v n=%d", err, n)
	}
	a.ResetStats()
	a.PlayOpus(payload[:n])
	if s := a.ReportAndReset(); !strings.Contains(s, "解码失败=0") {
		t.Fatalf("合法帧不应计解码失败: %s", s)
	}
}

func TestPortAudioClearPlaybackAndCallback(t *testing.T) {
	a, _ := newPortAudio()
	// 直接灌入播放缓冲（模拟已解码待播数据）
	a.playCh <- make([]int16, 100)
	a.pmu.Lock()
	a.playBuf = append(a.playBuf, make([]int16, audioRate)...)
	a.pmu.Unlock()

	a.ClearPlayback()
	select {
	case <-a.playCh:
		t.Fatal("ClearPlayback 应排空待播队列")
	default:
	}
	a.pmu.Lock()
	bufLen, started := len(a.playBuf), a.playStarted
	a.pmu.Unlock()
	if bufLen != 0 || started {
		t.Fatalf("ClearPlayback 应清缓冲并复位蓄水闸门: len=%d started=%v", bufLen, started)
	}

	// 蓄水前回调应输出静音（全零）
	out := make([]int16, 256)
	a.outCallback(out)
	for _, s := range out {
		if s != 0 {
			t.Fatal("蓄水前回调应输出零")
		}
	}

	// 蓄水达标后回调应输出数据并扣减缓冲
	a.pmu.Lock()
	a.playBuf = append(a.playBuf, make([]int16, audioRate*300/1000+16)...)
	a.pmu.Unlock()
	a.outCallback(out)
	if out[0] != 0 || out[len(out)-1] != 0 {
		t.Fatal("回调输出异常")
	}
	a.pmu.Lock()
	remaining := len(a.playBuf)
	a.pmu.Unlock()
	if remaining == 0 {
		t.Fatal("回调应消费缓冲")
	}
}

func TestPortAudioStopIdempotent(t *testing.T) {
	a, _ := newPortAudio()
	a.Stop()
	a.Stop() // 未启动时 Stop 应安全
}
