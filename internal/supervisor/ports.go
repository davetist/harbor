package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
)

// Reservations cover the interval before Vere binds and last only as long as
// the owned process. Saved ports on stopped ships do not reserve anything.
func (s *Supervisor) reserveHTTPPort(ctx context.Context, address string) (int, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for port := 80; port <= 65535; port++ {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		if !s.ports[port] {
			listener, err := net.Listen("tcp4", net.JoinHostPort(address, fmt.Sprint(port)))
			if err == nil {
				listener.Close()
				s.ports[port] = true
				var once sync.Once
				return port, func() {
					once.Do(func() {
						s.mu.Lock()
						delete(s.ports, port)
						s.mu.Unlock()
					})
				}, nil
			}
			if !errors.Is(err, syscall.EADDRINUSE) && !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) {
				return 0, nil, errors.New("ship HTTP address is unavailable")
			}
		}
		if port == 80 {
			port = 8079
		}
	}
	return 0, nil, errors.New("no available ship HTTP port")
}
