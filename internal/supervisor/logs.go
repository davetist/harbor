package supervisor

import (
	"context"
	"harbor/internal/model"
	"io"
	"sync"
)

const maxLogBytes = 1 << 20

type LogBuffer struct {
	mu      sync.Mutex
	data    []byte
	publish func([]byte)
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	n := len(p)
	if n >= maxLogBytes {
		b.data = append(b.data[:0], p[n-maxLogBytes:]...)
	} else {
		excess := len(b.data) + n - maxLogBytes
		if excess > 0 {
			copy(b.data, b.data[excess:])
			b.data = b.data[:len(b.data)-excess]
		}
		b.data = append(b.data, p...)
	}
	b.mu.Unlock()
	if b.publish != nil {
		b.publish(p)
	}
	return n, nil
}
func (b *LogBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }

func (s *Supervisor) LogWriter(id model.ID) *LogBuffer { return s.buffer(id) }

type jobWriter struct {
	supervisor *Supervisor
	job, ship  model.ID
}

func (w jobWriter) Write(p []byte) (int, error) {
	w.supervisor.buffer(w.ship).Write(p)
	if err := w.supervisor.store.AppendJobLog(context.Background(), w.job, p); err != nil {
		w.supervisor.mu.Lock()
		w.supervisor.logErrors[w.job] = true
		w.supervisor.mu.Unlock()
	}
	return len(p), nil // Never stop draining a live child's output on a disk error.
}
func (s *Supervisor) JobLogWriter(ship, job model.ID) io.Writer { return jobWriter{s, job, ship} }
func (s *Supervisor) JobLogAvailable(job model.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.logErrors[job]
}
