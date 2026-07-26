// SPDX-License-Identifier: MIT

package httpserver

import (
	"context"
	"net"
	"net/http"
	"time"
)

const (
	// DefaultReadHeaderTimeout bounds slow-dripped request headers without
	// limiting long-lived response streams such as SSE and chat completions.
	DefaultReadHeaderTimeout = 10 * time.Second
	// DefaultIdleTimeout bounds idle keep-alive connections.
	DefaultIdleTimeout = 120 * time.Second
	// DefaultShutdownTimeout bounds graceful drain after daemon cancellation.
	DefaultShutdownTimeout = 3 * time.Second
)

// ErrorHandler receives an unexpected serving error. http.ErrServerClosed is
// consumed by Start because it is the expected result of graceful shutdown.
type ErrorHandler func(error)

// NewStreamingServer builds the hardened server shared by daemon HTTP
// surfaces. WriteTimeout intentionally stays unset: a global write deadline
// would terminate legitimate SSE and streaming completion responses.
func NewStreamingServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		IdleTimeout:       DefaultIdleTimeout,
	}
}

// Start serves handler on an already-bound listener and ties graceful shutdown
// to ctx. Binding, fallback addresses, TLS, and surface-specific error wording
// remain with the caller.
func Start(
	ctx context.Context,
	listener net.Listener,
	handler http.Handler,
	onError ErrorHandler,
) *http.Server {
	if ctx == nil {
		panic("httpserver: nil context")
	}
	if listener == nil {
		panic("httpserver: nil listener")
	}
	if handler == nil {
		panic("httpserver: nil handler")
	}

	server := NewStreamingServer(handler)
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed && onError != nil {
			onError(err)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-serveDone:
		}
	}()
	return server
}
