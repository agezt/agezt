// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"errors"
	apppulse "github.com/agezt/agezt/kernel/app/pulse"
	"net"
	"time"
)

type pulseNativeConnKey struct{}

// handlePulseAppStream projects only the native event envelopes: a bounded or
// clean stream closes without RespResult. Its single timeout-tolerant watcher
// begins after replay, so generic cancelOnConnClose must not also read the conn.
func handlePulseAppStream(dc *DispatchCtx) {
	ctx, cancel := context.WithCancel(dc.Ctx)
	defer cancel()
	ctx = context.WithValue(ctx, pulseNativeConnKey{}, dc.Conn)
	_, err := dispatchAppOperation(dc, ctx, pulseNativeEmitter{appEmitter{dc.Conn, dc.Req.ID}})
	if err != nil {
		dc.S.fail(dc.Conn, dc.Req, err)
	}
}

// Unlimited native replay historically delegates cancellation to the socket
// writer. Preserve that convention; rate waits/live lifetime still use ctx.
type pulseNativeEmitter struct{ appEmitter }

func (e pulseNativeEmitter) Emit(ctx context.Context, value any) error {
	return e.appEmitter.Emit(context.WithoutCancel(ctx), value)
}

func pulseClientGone(ctx context.Context, conn net.Conn) <-chan struct{} {
	clientGone := make(chan struct{})
	stopCh := ctx.Done()
	go func() {
		var buf [1]byte
		for {
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			_, err := conn.Read(buf[:])
			if err == nil {
				// Should not happen — Pulse never sends client-side
				// data — but a stray byte is not a disconnect either;
				// just loop and keep watching.
				select {
				case <-stopCh:
					close(clientGone)
					return
				default:
				}
				continue
			}
			// Read-timeout: idle client, not a disconnect.
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				select {
				case <-stopCh:
					close(clientGone)
					return
				default:
					continue
				}
			}
			// Any other error (EOF, ErrClosed, network reset) means the
			// client actually went away.
			close(clientGone)
			return
		}
	}()

	return clientGone
}

func (s *Server) pulseStream() *apppulse.Stream {
	return apppulse.NewStream(s.k.Journal(), func(pattern string, buffer int) (apppulse.Subscription, error) {
		sub, err := s.k.Bus().Subscribe(pattern, buffer)
		if err != nil {
			return apppulse.Subscription{}, err
		}
		return apppulse.Subscription{Events: sub.C, Dropped: sub.Dropped.Load, Cancel: sub.Cancel}, nil
	}, nil)
}
