package exec

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Event is one line in the execution JSONL log.
type Event struct {
	Seq       int64  `json:"seq"`
	TS        int64  `json:"ts"`
	Type      string `json:"type"` // log | state | end
	Entity    string `json:"entity,omitempty"`
	Status    string `json:"status,omitempty"`
	HostRunID string `json:"host_run,omitempty"`
	StepRunID string `json:"step_run,omitempty"`
	Stream    string `json:"stream,omitempty"` // stdout | stderr
	Data      string `json:"data,omitempty"`
	Message   string `json:"message,omitempty"`
}

// EventLog appends events to a JSONL file and broadcasts them to the hub.
type EventLog struct {
	mu     sync.Mutex
	file   *os.File
	seq    int64
	hub    *Hub
	execID string
}

// OpenEventLog creates or appends the JSONL file for an execution.
func OpenEventLog(path, execID string, hub *Hub) (*EventLog, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &EventLog{file: f, hub: hub, execID: execID}, nil
}

// Emit appends the event and broadcasts it. Seq and TS are assigned here.
func (l *EventLog) Emit(ev Event) {
	l.mu.Lock()
	l.seq++
	ev.Seq = l.seq
	ev.TS = time.Now().UnixMilli()
	b, err := json.Marshal(ev)
	if err == nil {
		_, _ = l.file.Write(append(b, '\n'))
	}
	l.mu.Unlock()
	if l.hub != nil {
		l.hub.Broadcast(l.execID, ev)
	}
}

// Close flushes and closes the log file.
func (l *EventLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// ReadEvents reads events with seq > afterSeq, up to limit (limit <= 0 means no
// limit).
func ReadEvents(path string, afterSeq int64, limit int) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	out := []Event{}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Seq <= afterSeq {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RenderText converts events into a human-readable log for download.
func RenderText(events []Event, hostRunID, stepRunID string) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Type != "log" {
			continue
		}
		if hostRunID != "" && ev.HostRunID != hostRunID {
			continue
		}
		if stepRunID != "" && ev.StepRunID != stepRunID {
			continue
		}
		tag := ev.Stream
		if tag == "" {
			tag = "stdout"
		}
		fmt.Fprintf(&b, "[%s] [%s] %s", ev.HostRunID, tag, ev.Data)
		if !strings.HasSuffix(ev.Data, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
