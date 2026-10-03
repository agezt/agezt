// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDispatch_PanickingOpIsJournaledAsFailed: a state-changing handler that
// panics is recorded as op.failed. The audit trail must not be left with an
// invocation and no outcome, nor claim a success it cannot see.
func TestDispatch_PanickingOpIsJournaledAsFailed(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"

	const cmd = "zz_test_panics"
	commandRegistry[cmd] = commandSpec{Cmd: cmd, Handler: func(*DispatchCtx) { panic("handler bug") }}
	t.Cleanup(func() { delete(commandRegistry, cmd) })

	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), server) }()

	line, _ := json.Marshal(Request{ID: "1", Cmd: cmd, Token: "primary"})
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(client).ReadBytes('\n'); err != nil {
		t.Fatalf("reading the error response: %v", err)
	}
	<-done

	var got []event.Kind
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "op."+cmd {
			got = append(got, e.Kind)
		}
		return nil
	})
	if len(got) != 2 || got[0] != event.KindOpInvoked || got[1] != event.KindOpFailed {
		t.Fatalf("op events = %v, want [op.invoked op.failed]", got)
	}
}
