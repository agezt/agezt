// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBoardAppSocketRequiresAuditBeforeStoreAndNotifier(t *testing.T) {
	for _, cmd := range []string{CmdBoardSend, CmdBoardAck} {
		t.Run(cmd, func(t *testing.T) {
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			store, err := board.Open(filepath.Join(dir, "board"))
			if err != nil {
				t.Fatal(err)
			}
			seed, err := store.Send(board.Message{Topic: "dm", From: "writer", To: "reviewer", Text: "owned"}, 100)
			if err != nil {
				t.Fatal(err)
			}
			before := store.Read("", 0)
			notifications := 0
			s := NewServer(k, dir)
			s.token = "primary"
			s.SetBoard(store, func(board.Message, string) { notifications++ })
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			r := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": seed.ID, "by": "reviewer", "topic": "status", "text": "owned new", "correlation_id": "inbound"}})[0]
			after := store.Read("", 0)
			if r.Type != RespError || !strings.Contains(r.Error, "journal") || !reflect.DeepEqual(before, after) || notifications != 0 {
				t.Fatalf("EXPECTED: unavailable audit => error, unchanged board, no notifier; ACTUAL: command=%s response=%+v changed=%v notify=%d", cmd, r, !reflect.DeepEqual(before, after), notifications)
			}
		})
	}
}

func TestBoardCommandMetadataComesFromCompleteAppRegistry(t *testing.T) {
	if len(boardOperations) != 7 {
		t.Fatalf("board operations=%d", len(boardOperations))
	}
	reads := map[string]bool{CmdBoardRead: true, CmdBoardHelp: true, CmdBoardInbox: true, CmdBoardGet: true, CmdBoardReplies: true}
	for _, op := range boardOperations {
		spec := op.Spec()
		wire := commandRegistry[spec.Name]
		if !wire.AppOwned || wire.ReadOnly != reads[spec.Name] || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("board metadata=%+v", wire)
		}
	}
}

// TestBoardAppHostRefusesWithoutABoardAndWritesThroughTheSharedOne: the
// control plane's board surface answers the same way in both directions when
// no board is wired — "the board is not available on this daemon" — and the
// shared instance is what makes the writes work.
func TestBoardAppHostRefusesWithoutABoardAndWritesThroughTheSharedOne(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	store, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := store.Send(board.Message{Topic: "dm", From: "writer", To: "reviewer", Text: "owned"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	// Unwired: the board is not available — for the reads AND the writes. The
	// control plane does not open the board's file itself (W3.59): the daemon
	// owns that file and hands over the port, so a board that is not there
	// says so instead of answering an empty board.
	for _, cmd := range []string{CmdBoardRead, CmdBoardHelp, CmdBoardInbox, CmdBoardGet, CmdBoardReplies} {
		r := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": m.ID, "to": "reviewer"}})[0]
		if r.Type != RespError || !strings.Contains(r.Error, "not available") {
			t.Fatalf("unwired read %s=%+v", cmd, r)
		}
	}
	for _, cmd := range []string{CmdBoardSend, CmdBoardAck} {
		r := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": m.ID, "by": "reviewer", "text": "owned", "topic": "status"}})[0]
		if r.Type != RespError || !strings.Contains(r.Error, "not available") {
			t.Fatalf("unwired writer %s=%+v", cmd, r)
		}
	}
	calls := 0
	corr := ""
	s.SetBoard(store, func(_ board.Message, c string) { calls++; corr = c })
	r := callAppHost(t, s, Request{ID: "send", Cmd: CmdBoardSend, Token: "primary", Args: map[string]any{"text": " new ", "topic": " status ", "correlation_id": " inbound ", "unused": true}})[0]
	if r.Type != RespResult || r.Result["correlation_id"] != "inbound" || calls != 1 || corr != "inbound" || len(store.Read("", 0)) != 2 {
		t.Fatalf("shared send=%+v notify=%d/%q", r, calls, corr)
	}
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{CmdBoardRead, CmdBoardHelp, CmdBoardInbox, CmdBoardGet, CmdBoardReplies} {
		r := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": m.ID, "to": "reviewer"}})[0]
		if r.Type != RespResult {
			t.Fatalf("read unexpectedly audited %s=%+v", cmd, r)
		}
	}
}
