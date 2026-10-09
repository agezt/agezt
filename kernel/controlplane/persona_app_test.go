// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPersonaBindsTheServer: the default identity applies to the primary kernel
// and persists, with the prompt library, under the server's base directory, not
// the kernel's.
func TestPersonaBindsTheServer(t *testing.T) {
	kernelDir, serverDir := t.TempDir(), t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: kernelDir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, serverDir)
	s.token = "primary"
	call := func(cmd string, args map[string]any) string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "p", Cmd: cmd, Token: "primary", Args: args})[0]
		if resp.Type != RespResult {
			t.Fatal(cmd, resp.Error)
		}
		raw, _ := json.Marshal(resp.Result)
		return string(raw)
	}
	if got := call(CmdPersonaSet, map[string]any{"system": "be brief"}); got != `{"applied":"live","length":8,"saved":true,"set":true}` {
		t.Fatal(got)
	}
	if k.System() != "be brief" {
		t.Fatal("applied live to the primary kernel", k.System())
	}
	store := settings.NewStore(serverDir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get("AGEZT_SYSTEM_PROMPT"); got != "be brief" {
		t.Fatal("persisted under the server directory", got)
	}
	if got := call(CmdPromptsSet, map[string]any{"prompts": []any{map[string]any{"title": "a", "text": "b"}}}); got != `{"count":1,"saved":true}` {
		t.Fatal(got)
	}
	info, err := os.Stat(filepath.Join(serverDir, "chat_prompts.json"))
	if err != nil {
		t.Fatal("the library lives under the server directory", err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatal("the library is private to the owner", mode)
	}
	if _, err := os.Stat(filepath.Join(kernelDir, "chat_prompts.json")); err == nil {
		t.Fatal("nothing is written under the kernel directory")
	}
	if got := call(CmdPromptsGet, nil); got != `{"prompts":[{"text":"b","title":"a"}]}` {
		t.Fatal(got)
	}
	for cmd, read := range map[string]bool{CmdPersonaGet: true, CmdPersonaSet: false, CmdPromptsGet: true, CmdPromptsSet: false} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}
