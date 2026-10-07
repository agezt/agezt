// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestChannelSendNativeContextLivesThroughTerminalWrite(t *testing.T) {
	for _, senderError := range []error{nil, errors.New("owned sender error")} {
		s := &Server{}
		var senderContext context.Context
		s.channelSend = func(ctx context.Context, kind, to, text string) error { senderContext = ctx; return senderError }
		client, conn := net.Pipe()
		client.SetDeadline(time.Now().Add(3 * time.Second))
		entered, release := make(chan struct{}), make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.handleSend(pausedOAuthStatusConn{Conn: conn, entered: entered, release: release}, Request{ID: "owned", Args: map[string]any{"channel": "Slack", "to": "owned", "text": "fixture"}})
		}()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("send did not reach terminal writer")
		}
		if senderContext == nil || senderContext.Err() != nil {
			close(release)
			client.Close()
			conn.Close()
			<-done
			t.Fatal("sender context canceled before terminal write", senderContext)
		}
		close(release)
		if _, err := bufio.NewReader(client).ReadBytes(10); err != nil {
			t.Fatal(err)
		}
		client.Close()
		conn.Close()
		<-done
		if !errors.Is(senderContext.Err(), context.Canceled) {
			t.Fatal("sender context not released after write", senderContext.Err())
		}
	}
}
