// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestWebhookNativeCanceledBeforeJournal(t *testing.T) {
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: p})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	for _, cmd := range []string{CmdWebhookLog, CmdWebhookStats} {
		t.Run(cmd, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			a.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan struct{})
			go func() { defer close(done); s.handleConn(ctx, b) }()
			raw, _ := json.Marshal(Request{ID: "owned-webhook", Cmd: cmd, Token: "primary"})
			if _, err := a.Write(append(raw, 10)); err != nil {
				t.Fatal(err)
			}
			line, err := bufio.NewReader(a).ReadBytes(10)
			a.Close()
			<-done
			if err != nil {
				t.Fatal(err)
			}
			var reply Response
			if err := json.Unmarshal(line, &reply); err != nil {
				t.Fatal(err)
			}
			if reply.Type != RespError || reply.Error != "context canceled" || p.CallCount() != 0 {
				t.Fatalf("EXPECTED: canceled native read rejected; ACTUAL: %s", line)
			}
		})
	}
}
