// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
	"time"
)

func TestTasteCommandMetadataComesFromCompleteAppRegistry(t *testing.T) {
	if len(tasteOperations) != 3 {
		t.Fatalf("taste operations=%d", len(tasteOperations))
	}
	for _, op := range tasteOperations {
		spec := op.Spec()
		wire := commandRegistry[spec.Name]
		if !wire.AppOwned || wire.ReadOnly != (spec.Name == CmdTasteList) || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("taste wire=%+v", wire)
		}
	}
}
func TestTasteAppSocketRequiresAuditBeforeMutations(t *testing.T) {
	for _, cmd := range []string{CmdTasteCreate, CmdTasteDelete} {
		t.Run(cmd, func(t *testing.T) {
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			e, err := k.Taste().Create(taste.CreateSpec{Title: "seed", Body: "body"}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			s := NewServer(k, dir)
			s.token = "primary"
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			r := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"title": "new", "body": "body", "id": e.ID}})[0]
			all := k.Taste().List(taste.Filter{})
			if r.Type != RespError || !strings.Contains(r.Error, "journal") || len(all) != 1 || all[0].ID != e.ID {
				t.Fatalf("audit precedes effect: cmd=%s response=%+v exemplars=%+v", cmd, r, all)
			}
			r = callAppHost(t, s, Request{ID: "read", Cmd: CmdTasteList, Token: "primary"})[0]
			if r.Type != RespResult {
				t.Fatalf("read unexpectedly audited: %+v", r)
			}
		})
	}
}
func TestTasteNativeMutationHasOneOwnedAuditArc(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	r := callAppHost(t, s, Request{ID: "create", Cmd: CmdTasteCreate, Token: "primary", Args: map[string]any{"title": "fixture", "body": "body"}})[0]
	if r.Type != RespResult {
		t.Fatal(r)
	}
	var invoked, completed []*event.Event
	if err := k.Journal().Range(func(e *event.Event) error {
		copy := *e
		if e.Subject == "op.taste_create" {
			if e.Kind == event.KindOpInvoked {
				invoked = append(invoked, &copy)
			}
			if e.Kind == event.KindOpCompleted {
				completed = append(completed, &copy)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(invoked) != 1 || len(completed) != 1 || invoked[0].CorrelationID == "" || invoked[0].CorrelationID != completed[0].CorrelationID || invoked[0].Seq >= completed[0].Seq {
		t.Fatalf("audit arc=%v/%v", invoked, completed)
	}
}
