// SPDX-License-Identifier: MIT

package controlplane

import (
	"regexp"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestReflectionRunsOnThePrimaryKernel: each pass gets a fresh reflect-<ulid>
// correlation, journals its report under it in the primary kernel, and the
// latest report reads back.
func TestReflectionRunsOnThePrimaryKernel(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	corr := func() string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "r", Cmd: CmdReflectRun, Token: "primary"})[0]
		if resp.Type != RespResult {
			t.Fatal(resp.Error)
		}
		c, _ := resp.Result["correlation_id"].(string)
		if !regexp.MustCompile(`^reflect-[0-9A-Z]{26}$`).MatchString(c) {
			t.Fatal("a reflect-<ulid> correlation", c)
		}
		return c
	}
	first, second := corr(), corr()
	if first == second {
		t.Fatal("each pass mints its own correlation")
	}
	journaled := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID == second {
			journaled++
		}
		return nil
	})
	if journaled == 0 {
		t.Fatal("the pass journals under its correlation in the primary kernel")
	}
	resp := callAppHost(t, s, Request{ID: "s", Cmd: CmdReflectShow, Token: "primary"})[0]
	if found, _ := resp.Result["found"].(bool); !found {
		t.Fatal("the latest report reads back", resp)
	}
	for cmd, read := range map[string]bool{CmdReflectRun: false, CmdReflectShow: true} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}
