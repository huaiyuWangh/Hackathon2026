package ccrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// 状态持久化：--local 模式下 send/status 分属不同进程，
// 任务与采样基线落在工作目录 .bridge/state.json 共享。

type persistedTask struct {
	ID      string    `json:"id"`
	Prompt  string    `json:"prompt"`
	State   string    `json:"state"`
	Summary string    `json:"summary"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	SawBusy bool      `json:"saw_busy"`
}

type persistedState struct {
	Seq        int                      `json:"seq"`
	LastID     string                   `json:"last_id"`
	LastANSI   string                   `json:"last_ansi"`
	LastPlain  string                   `json:"last_plain"`
	LastSample time.Time                `json:"last_sample"`
	Tasks      map[string]persistedTask `json:"tasks"`
}

func (r *TmuxRunner) statePath() string {
	return filepath.Join(r.workDir, ".bridge", "state.json")
}

// loadState 装载历史状态；文件缺失/损坏时静默从零开始。
func (r *TmuxRunner) loadState() {
	raw, err := os.ReadFile(r.statePath())
	if err != nil {
		return
	}
	var ps persistedState
	if err := json.Unmarshal(raw, &ps); err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq = ps.Seq
	r.lastANSI, r.lastPlain, r.lastSampleAt = ps.LastANSI, ps.LastPlain, ps.LastSample
	for id, pt := range ps.Tasks {
		r.tasks[id] = &task{
			id: pt.ID, prompt: pt.Prompt, state: pt.State, summary: pt.Summary,
			started: pt.Started, ended: pt.Ended, sawBusy: pt.SawBusy,
		}
	}
	if t, ok := r.tasks[ps.LastID]; ok {
		r.last = t
	}
}

// saveState 原子写全量状态（tmp+rename 防半写）。
func (r *TmuxRunner) saveState() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveStateLocked()
}

// saveStateLocked 仅在已持有 r.mu 时调用。
func (r *TmuxRunner) saveStateLocked() {
	ps := persistedState{
		Seq:        r.seq,
		LastANSI:   r.lastANSI,
		LastPlain:  r.lastPlain,
		LastSample: r.lastSampleAt,
		Tasks:      make(map[string]persistedTask, len(r.tasks)),
	}
	if r.last != nil {
		ps.LastID = r.last.id
	}
	for id, t := range r.tasks {
		ps.Tasks[id] = persistedTask{
			ID: t.id, Prompt: t.prompt, State: t.state, Summary: t.summary,
			Started: t.started, Ended: t.ended, SawBusy: t.sawBusy,
		}
	}
	raw, err := json.Marshal(ps)
	if err != nil {
		return
	}
	dir := filepath.Dir(r.statePath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp := r.statePath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, r.statePath())
}
