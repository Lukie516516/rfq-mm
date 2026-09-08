package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Event is one immutable desk decision. Replay these, do not trust memory.
type Event struct {
	Ts      time.Time      `json:"ts"`
	Kind    string         `json:"kind"`
	RoundID string         `json:"round_id,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type Writer interface {
	Write(Event) error
}

type JSONL struct {
	mu   sync.Mutex
	path string
}

func NewJSONL(path string) *JSONL {
	return &JSONL{path: path}
}

func (j *JSONL) Write(e Event) error {
	if j == nil || j.path == "" {
		return nil
	}
	if e.Ts.IsZero() {
		e.Ts = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

type Buffer struct {
	Events []Event
}

func (b *Buffer) Write(e Event) error {
	if e.Ts.IsZero() {
		e.Ts = time.Now().UTC()
	}
	b.Events = append(b.Events, e)
	return nil
}
