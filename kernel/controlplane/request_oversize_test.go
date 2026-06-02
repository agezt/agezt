// SPDX-License-Identifier: MIT

package controlplane_test

// Live proof for M188: the control plane reads the request line BEFORE
// authentication, so an unauthenticated local client that floods bytes
// without a newline must be rejected (bounded), not allowed to OOM the
// daemon. We dial the listener raw and stream >16 MiB with no newline;
// the server must answer "request too large" and not hang.

import (
	"bufio"
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestServer_RejectsOversizedPreAuthRequest(t *testing.T) {
	_, srv, _, _ := startPair(t, mock.New(mock.FinalText("ok")))

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))

	// Stream 17 MiB with no newline (no valid token ever sent), ignoring
	// write errors — the server closes once it hits the 16 MiB cap.
	go func() {
		chunk := bytes.Repeat([]byte("x"), 1<<20)
		for i := 0; i < 17; i++ {
			if _, werr := conn.Write(chunk); werr != nil {
				return
			}
		}
	}()

	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("expected a 'request too large' response, got read err: %v", err)
	}
	if !strings.Contains(resp, "too large") {
		t.Errorf("response = %q; want it to mention 'too large'", resp)
	}
}
