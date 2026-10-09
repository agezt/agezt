// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRedactNativeTypedRegistry(t *testing.T) {
	found := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdRedactTest {
			continue
		}
		found++
		if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("metadata=%+v", spec)
		}
	}
	wire, exists := commandRegistry[CmdRedactTest]
	if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("native wire found=%d metadata=%+v", found, wire)
	}
}

// The check reads the bus's live redactor at call time and never journals the
// candidate.
func TestRedactNativeLiveRedactor(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	call := func(args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "r", Cmd: CmdRedactTest, Token: "primary", Args: args})
		if _, err := a.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(a).ReadBytes(10)
		a.Close()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		return string(line)
	}
	literal := "native-" + "literal-" + "value"
	if line := call(map[string]any{"text": literal}); !strings.Contains(line, `"enabled":false`) {
		t.Fatal(line)
	}
	red := redact.New()
	red.SetSecrets([]string{literal})
	k.Bus().SetRedactor(red)
	head, _ := k.Journal().Head()
	line := call(map[string]any{"text": "pw " + literal})
	if !strings.Contains(line, `"enabled":true`) || !strings.Contains(line, `"literal_hit":true`) || strings.Contains(line, literal) {
		t.Fatal("a redactor installed after boot is used, and the secret never comes back", line)
	}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Seq > head && strings.Contains(string(e.Payload), "native-literal") {
			t.Fatalf("the candidate was journaled: %+v", e)
		}
		return nil
	})
}
