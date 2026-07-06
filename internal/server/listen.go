package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

// Listen binds a TCP listener on the loopback interface only. The server writes
// files, so it must never be reachable from another host by default
// (DESIGN.md §7, test row 9).
func Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("server: binding 127.0.0.1:%d: %w", port, err)
	}
	return ln, nil
}

// Serve starts watching the project and serves HTTP on ln until ctx is
// cancelled, then shuts down gracefully.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if err := s.Watch(ctx); err != nil {
		return err
	}
	srv := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
