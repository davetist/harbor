package server

import (
	"encoding/json"
	"fmt"
	"harbor/internal/model"
	"net/http"
	"sync"
	"time"
)

type Broker struct {
	mu      sync.Mutex
	next    uint64
	clients map[chan model.Event]struct{}
}

func NewBroker() *Broker { return &Broker{clients: make(map[chan model.Event]struct{})} }
func (b *Broker) Subscribe() (<-chan model.Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan model.Event, 128)
	b.clients[ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.clients[ch]; ok {
			delete(b.clients, ch)
			close(ch)
		}
	}
}
func (b *Broker) Publish(e model.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	e.ID = b.next
	for ch := range b.clients {
		select {
		case ch <- e:
		default:
			delete(b.clients, ch)
			close(ch)
		}
	}
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if _, ok := w.(http.Flusher); !ok {
		fail(w, 500, "stream", "Streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, cancel := s.Events.Subscribe()
	defer cancel()
	rc := http.NewResponseController(w)
	send := func(e model.Event) error {
		rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b); err != nil {
			return err
		}
		return rc.Flush()
	}
	// Durable state is always authoritative; no unlimited log replay is promised.
	if send(model.Event{Type: "resync"}) != nil {
		return
	}
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if send(e) != nil {
				return
			}
		case <-tick.C:
			rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		}
	}
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		delete(b.clients, ch)
		close(ch)
	}
}
