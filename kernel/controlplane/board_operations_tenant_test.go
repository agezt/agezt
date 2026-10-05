// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoardOperationsSocketRetainPrimarySharedStore(t *testing.T) {
	_, server, owner, dir := startPair(t, mock.New(mock.FinalText("unused")))
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	store, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := store.Send(board.Message{Topic: "status", From: "writer", To: "reviewer", Text: "primary-private"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server.SetBoard(store, func(board.Message, string) { calls++ })
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdBoardRead, controlplane.CmdBoardHelp, controlplane.CmdBoardInbox, controlplane.CmdBoardGet, controlplane.CmdBoardReplies, controlplane.CmdBoardSend, controlplane.CmdBoardAck} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "id": seed.ID, "by": "reviewer", "to": "reviewer", "text": "blocked", "topic": "status"}); err == nil {
			t.Fatalf("tenant admitted %s", cmd)
		}
	}
	if calls != 0 || len(store.Read("", 0)) != 1 {
		t.Fatal("denied tenant changed shared board")
	}
	result, err := owner.Call(context.Background(), controlplane.CmdBoardRead, map[string]any{"tenant": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(wire), "primary-private") {
		t.Fatalf("primary selected board=%s err=%v", wire, err)
	}
	if _, err := owner.Call(context.Background(), controlplane.CmdBoardSend, map[string]any{"tenant": "acme", "topic": "status", "text": "primary-create"}); err != nil {
		t.Fatal(err)
	}
	if len(store.Read("", 0)) != 2 || calls != 1 {
		t.Fatalf("operator selected wrong writer or notifier count=%d notify=%d", len(store.Read("", 0)), calls)
	}
}
