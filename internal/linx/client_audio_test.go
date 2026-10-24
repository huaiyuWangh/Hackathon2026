package linx

import (
	"strings"
	"testing"
)

// fakeAudio 记录调用的音频设备桩：验证 Client 对 TTS 生命周期的编排。
type fakeAudio struct {
	muted, speaking bool
	reset, report   int
	clear           int
	played          [][]byte
}

func (f *fakeAudio) Start(func([]byte)) error { return nil }
func (f *fakeAudio) Stop()                    {}
func (f *fakeAudio) PlayOpus(b []byte)        { f.played = append(f.played, b) }
func (f *fakeAudio) ClearPlayback()           { f.clear++ }
func (f *fakeAudio) SetMuted(m bool)          { f.muted = m }
func (f *fakeAudio) Muted() bool              { return f.muted }
func (f *fakeAudio) SetSpeaking(on bool)      { f.speaking = on }
func (f *fakeAudio) ResetStats()              { f.reset++ }
func (f *fakeAudio) ReportAndReset() string   { f.report++; return "stats" }

func TestClientMicControl(t *testing.T) {
	// 未启用音频：控制与查询都应安全失败/返回 false
	cli := New(nil, fakeProvider{})
	if _, err := cli.SetMic(true); err == nil || !strings.Contains(err.Error(), "音频未启用") {
		t.Fatalf("无音频 SetMic 应报错: %v", err)
	}
	if cli.MicOn() {
		t.Fatal("无音频 MicOn 应为 false")
	}

	// 注入 fake 设备：开关切换生效
	cli.audio = &fakeAudio{muted: true}
	if on, err := cli.SetMic(true); err != nil || !on {
		t.Fatalf("SetMic(true) 失败: %v %v", on, err)
	}
	if !cli.MicOn() {
		t.Fatal("开启后 MicOn 应为 true")
	}
	if on, _ := cli.SetMic(false); on {
		t.Fatal("SetMic(false) 应关闭")
	}
}

func TestHandleServerMessageTTSLifecycle(t *testing.T) {
	cli := New(nil, fakeProvider{})
	a := &fakeAudio{}
	cli.audio = a

	// 非 JSON / 未知类型 / 无音频设备：安全忽略
	cli.handleServerMessage([]byte("not-json"))
	cli.handleServerMessage([]byte(`{"type":"stt","payload":{"text":"hi"}}`))

	cli.handleServerMessage([]byte(`{"type":"tts","state":"start"}`))
	if a.reset != 1 || !a.speaking {
		t.Fatalf("tts start 应清零统计并开启回声门控: reset=%d speaking=%v", a.reset, a.speaking)
	}

	// 被打断的 stop：清播放缓冲 + 统计报告
	cli.handleServerMessage([]byte(`{"type":"tts","state":"stop","is_aborted":true}`))
	if a.speaking || a.clear != 1 || a.report != 1 {
		t.Fatalf("aborted stop 应关门控/清缓冲/出统计: speaking=%v clear=%d report=%d",
			a.speaking, a.clear, a.report)
	}

	// 正常 stop：不清缓冲
	cli.handleServerMessage([]byte(`{"type":"tts","state":"stop"}`))
	if a.clear != 1 {
		t.Fatalf("正常 stop 不应清缓冲: clear=%d", a.clear)
	}
}

func TestUplinkDispatch(t *testing.T) {
	cli := New(nil, fakeProvider{})
	var got []byte
	cli.setUplink(func(b []byte) { got = b })
	cli.uplink([]byte("frame"))
	if string(got) != "frame" {
		t.Fatalf("上行帧应送达当前连接回调: %q", got)
	}
	cli.setUplink(nil) // 无连接（重连间隙）：丢弃不 panic
	cli.uplink([]byte("dropped"))
	if string(got) != "frame" {
		t.Fatalf("无连接时不应改动: %q", got)
	}
}
