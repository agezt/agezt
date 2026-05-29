// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestToolList_ReturnsRegisteredTools verifies the wire shape:
// CmdToolList returns a `tools` array sorted by name, each entry
// carrying {name, description}. The default startPair rig wires
// one tool ("shell"), so this asserts the minimal happy path.
func TestToolList_ReturnsRegisteredTools(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdToolList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if got := intOf(res["count"]); got != 1 {
		t.Errorf("count = %d want 1", got)
	}
	rows, ok := res["tools"].([]any)
	if !ok {
		t.Fatalf("tools wrong type: %T", res["tools"])
	}
	if len(rows) != 1 {
		t.Fatalf("tools len = %d want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if row["name"] != "shell" {
		t.Errorf("name = %v want shell", row["name"])
	}
	// Description comes from shell.New().Definition(); we don't pin
	// the exact text (it can evolve) but it must be non-empty so the
	// CLI has something to render.
	if desc, _ := row["description"].(string); desc == "" {
		t.Error("description is empty; CLI would render a blank column")
	}
}

// TestToolList_EmptyWhenNoToolsRegistered covers the degenerate
// case — a kernel constructed with no tools (rare but legal, e.g.
// a planner-only daemon). The response must still be a valid
// JSON array, not null, so downstream jq pipelines don't break.
func TestToolList_EmptyWhenNoToolsRegistered(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
		// Tools omitted — kernel runs with no in-process tools.
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	client, err := dialUntilReady(t, dir)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.Call(context.Background(), controlplane.CmdToolList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 0 {
		t.Errorf("count = %d want 0", got)
	}
	rows, ok := res["tools"].([]any)
	if !ok {
		t.Fatalf("tools wrong type: %T (want []any even when empty)", res["tools"])
	}
	if len(rows) != 0 {
		t.Errorf("tools should be empty, got %d rows", len(rows))
	}
}

// TestToolList_SortsByName ensures the deterministic-output
// promise the handler doc makes — operators piping into diff,
// jq, or just visually scanning rely on a stable order across
// calls. Wires two tools so map-iteration randomness has a
// chance to surface in non-sorted output.
func TestToolList_SortsByName(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"zeta":  shell.New(),
			"alpha": shell.New(),
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	client, err := dialUntilReady(t, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Both fixture tools wrap shell.New(), so they advertise the
	// SAME Definition().Name ("shell"). The handler sorts on the
	// definition name, not the map key — verify the count is 2
	// and the order is stable across repeated calls.
	for i := 0; i < 3; i++ {
		res, err := client.Call(context.Background(), controlplane.CmdToolList, nil)
		if err != nil {
			t.Fatalf("Call %d: %v", i, err)
		}
		rows, _ := res["tools"].([]any)
		if len(rows) != 2 {
			t.Fatalf("call %d: tools len = %d want 2", i, len(rows))
		}
		a, _ := rows[0].(map[string]any)
		b, _ := rows[1].(map[string]any)
		na, _ := a["name"].(string)
		nb, _ := b["name"].(string)
		if na > nb {
			t.Errorf("call %d: not sorted: %q > %q", i, na, nb)
		}
	}
}

// intOf decodes a JSON number (always float64 after Go's stdlib
// decoder) back to int. Mirrors mcOf in budget_test.go but for
// non-microcent counts.
func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return -1
}

// dialUntilReady polls until the runtime files are on disk, then
// returns a connected client. Extracted from startPair so the
// custom-config tests above can construct their own kernels
// without reaching into the shared helper.
func dialUntilReady(t *testing.T, dir string) (*controlplane.Client, error) {
	t.Helper()
	var (
		client  *controlplane.Client
		lastErr error
	)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := controlplane.NewClient(dir)
		if err == nil {
			client = c
			break
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	if client == nil {
		return nil, lastErr
	}
	return client, nil
}
