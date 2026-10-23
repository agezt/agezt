// SPDX-License-Identifier: MIT

package pluginhost

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pluginapi"
	"github.com/agezt/agezt/kernel/internal/testfixtures"
)

// TestTheSpawnerHandsTheChildExactlyTheRequestedEnvironment: the host side of
// the environment promise. The plugin side asks for a scrubbed environment
// plus the variables the operator granted (measured in the plugin's own
// test); the host side must hand the child EXACTLY that — no more (the
// daemon's secrets must not leak in) and no less (the child must be able to
// launch).
//
// The measurement is against a REAL child process: the probe plugin reports
// what its own environment holds.
func TestTheSpawnerHandsTheChildExactlyTheRequestedEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a plugin binary")
	}
	bin, err := testfixtures.BuildEnvProbePlugin(t.TempDir())
	if err != nil {
		t.Fatalf("build env probe plugin: %v", err)
	}

	env := []string{
		"PATH=" + pathEnv(t),
		"PROBE_GRANTED_TOKEN=granted-ok",
	}
	spawned, err := NewSpawner().Spawn(context.Background(), pluginapi.SpawnRequest{
		Path: bin,
		Env:  env,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer spawned.Close()

	tools := spawned.Tools("")
	tool, ok := tools["getenv"]
	if !ok {
		t.Fatalf("the probe plugin advertised no tools: %v", tools)
	}
	read := func(name string) string {
		in, _ := json.Marshal(map[string]string{"name": name})
		res, err := tool.Invoke(context.Background(), in)
		if err != nil || res.IsError {
			t.Fatalf("invoke getenv(%s): %v %+v", name, err, res)
		}
		return strings.TrimSpace(res.Output)
	}
	if got := read("OPENAI_API_KEY"); got != "" {
		t.Errorf("the child saw OPENAI_API_KEY=%q — the daemon's secrets leaked into a third-party process", got)
	}
	if got := read("AGEZT_VAULT_PASSPHRASE"); got != "" {
		t.Errorf("the child saw AGEZT_VAULT_PASSPHRASE=%q", got)
	}
	if got := read("PROBE_GRANTED_TOKEN"); got != "granted-ok" {
		t.Errorf("the granted variable = %q, want granted-ok", got)
	}
	if got := read("PATH"); got == "" {
		t.Error("the child lost PATH — the environment must keep ordinary launch variables")
	}
}

// TestTheSpawnerEnforcesTheBinaryPin: the pin is a SECURITY control (the
// operator's recorded digest of the plugin binary). The adapter must hand it
// to the host, which refuses a binary that does not match it — so a plugin
// binary swapped under the operator's feet never starts.
func TestTheSpawnerEnforcesTheBinaryPin(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a plugin binary")
	}
	bin, err := testfixtures.BuildEnvProbePlugin(t.TempDir())
	if err != nil {
		t.Fatalf("build env probe plugin: %v", err)
	}
	wrongPin := strings.Repeat("d", 64)
	_, err = NewSpawner().Spawn(context.Background(), pluginapi.SpawnRequest{
		Path:       bin,
		PinnedHash: wrongPin,
	})
	if err == nil {
		t.Fatal("a binary that does not match the operator's pin must not start")
	}
	if !strings.Contains(err.Error(), "pin") && !strings.Contains(err.Error(), "hash") {
		t.Errorf("the refusal should name the pin, got: %v", err)
	}
}

// pathEnv returns the PATH this test runs with.
func pathEnv(t *testing.T) string {
	t.Helper()
	for _, kv := range os.Environ() {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			return v
		}
	}
	t.Fatal("PATH is not set")
	return ""
}
