// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestFileRestoreUsesDaemonCatalogAndIgnoresCallerSnapshot(t *testing.T) {
	k, _, client, daemonHome := startPairWithConfig(t, runtime.Config{Provider: mock.New(), NewToolInvoker: apptools.NewInvoker})
	otherHome := t.TempDir()
	t.Setenv("AGEZT_HOME", otherHome)
	trustedTarget := filepath.Join(t.TempDir(), "trusted.txt")
	otherTarget := filepath.Join(t.TempDir(), "other.txt")
	for _, target := range []string{trustedTarget, otherTarget} {
		if err := os.WriteFile(target, []byte("current"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	checkpoint := func(target, bytes string) rollbackstore.Checkpoint {
		return rollbackstore.Checkpoint{ID: "same-id", Kind: rollbackstore.KindFile, SubjectID: target, Before: map[string]any{
			"abs_path": target, "exists": true, "content_b64": base64.StdEncoding.EncodeToString([]byte(bytes)),
		}}
	}
	trusted := checkpoint(trustedTarget, "trusted prior bytes")
	other := checkpoint(otherTarget, "caller snapshot bytes")
	daemonCatalog := filepath.Join(daemonHome, filepath.FromSlash(rollbackstore.RelativePath))
	otherCatalog := filepath.Join(otherHome, filepath.FromSlash(rollbackstore.RelativePath))
	if err := rollbackstore.WriteAt(daemonCatalog, rollbackstore.Catalog{Checkpoints: []rollbackstore.Checkpoint{trusted}}); err != nil {
		t.Fatal(err)
	}
	if err := rollbackstore.WriteAt(otherCatalog, rollbackstore.Catalog{Checkpoints: []rollbackstore.Checkpoint{other}}); err != nil {
		t.Fatal(err)
	}
	result, err := client.Call(context.Background(), controlplane.CmdFileRestore, map[string]any{
		"id": "same-id", "before": other.Before,
	})
	if err != nil || result["applied"] != true {
		t.Fatalf("restore=%v error=%v", result, err)
	}
	if data, err := os.ReadFile(trustedTarget); err != nil || string(data) != "trusted prior bytes" {
		t.Fatalf("trusted target=%q error=%v", data, err)
	}
	if data, err := os.ReadFile(otherTarget); err != nil || string(data) != "current" {
		t.Fatalf("caller target changed=%q error=%v", data, err)
	}
	if cat, err := rollbackstore.LoadAt(otherCatalog); err != nil || cat.Checkpoints[0].AppliedMS != 0 {
		t.Fatalf("caller catalog changed=%+v error=%v", cat, err)
	}
	if cat, err := rollbackstore.LoadAt(daemonCatalog); err != nil || cat.Checkpoints[0].AppliedMS <= 0 {
		t.Fatalf("daemon catalog not applied=%+v error=%v", cat, err)
	}
	for _, value := range []string{"trusted prior bytes", "caller snapshot bytes", base64.StdEncoding.EncodeToString([]byte("trusted prior bytes")), base64.StdEncoding.EncodeToString([]byte("caller snapshot bytes"))} {
		if err := k.Journal().Range(func(e *event.Event) error {
			if strings.Contains(string(e.Payload), value) {
				t.Errorf("snapshot leaked into %s", e.Kind)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(trustedTarget, []byte("after restore"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = client.Call(context.Background(), controlplane.CmdFileRestore, map[string]any{"id": "same-id"})
	if err != nil || result["applied"] != false || result["reason"] != "already applied" {
		t.Fatalf("repeat=%v error=%v", result, err)
	}
	if data, err := os.ReadFile(trustedTarget); err != nil || string(data) != "after restore" {
		t.Fatalf("repeat changed target=%q error=%v", data, err)
	}
}
