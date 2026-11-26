package exec

import "sync"

// Hub fans out execution events to SSE subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{}
}

// NewHub builds an empty Hub.
func NewHub() *Hub { return &Hub{subs: make(map[string]map[chan Event]struct{})} }

// Subscribe returns a channel of events for one execution and a cancel func.
func (h *Hub) Subscribe(execID string) (<-chan Event, func()) {
	ch := make(chan Event, 512)
	h.mu.Lock()
	if h.subs[execID] == nil {
		h.subs[execID] = make(map[chan Event]struct{})
	}
	h.subs[execID][ch] = struct{}{}
	h.mu.Unlock()

	once := sync.Once{}
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			if set, ok := h.subs[execID]; ok {
				if _, exists := set[ch]; exists {
					delete(set, ch)
					close(ch)
				}
				if len(set) == 0 {
					delete(h.subs, execID)
				}
			}
			h.mu.Unlock()
		})
	}
	return ch, cancel
}

// Broadcast delivers an event without blocking. Slow subscribers are closed and
// can reconnect with Last-Event-ID to replay from the log file.
func (h *Hub) Broadcast(execID string, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[execID] {
		select {
		case ch <- ev:
		default:
			delete(h.subs[execID], ch)
			close(ch)
		}
	}
}

// CloseExecution closes all subscribers of an execution.
func (h *Hub) CloseExecution(execID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[execID] {
		delete(h.subs[execID], ch)
		close(ch)
	}
	delete(h.subs, execID)
}
