// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
)

func rollbackFileFixture(t *testing.T, home, target string, existed bool) string {
	t.Helper()
	path := filepath.Join(home, filepath.FromSlash(rollbackCatalogRelativePath))
	cp := rollbackCheckpoint{ID: "restore-proof", Kind: rollbackCheckpointKindFile, Action: "file.write", SubjectID: target, Before: map[string]any{
		"abs_path": target, "exists": existed, "content_b64": base64.StdEncoding.EncodeToString([]byte("private snapshot proof bytes")), "mode_perm": 0o600,
	}}
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{cp}}); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRollbackFileRestoreHasPrivateCorrelatedAudit(t *testing.T) {
	for _, existed := range []bool{true, false} {
		for _, denied := range []bool{false, true} {
			name := map[bool]string{true: "content", false: "absence"}[existed] + "/" + map[bool]string{true: "deny", false: "allow"}[denied]
			t.Run(name, func(t *testing.T) {
				home := t.TempDir()
				target := filepath.Join(t.TempDir(), "target.txt")
				if err := os.WriteFile(target, []byte("current bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
				catalog := rollbackFileFixture(t, home, target, existed)
				s, k := newFilesServerAt(t, home)
				capability := edict.CapFileWrite
				if !existed {
					capability = edict.CapFileDelete
				}
				if denied {
					k.Edict().SetLevel(capability, edict.LevelDeny)
				}
				sub, err := k.Bus().Subscribe("op.>", 16)
				if err != nil {
					t.Fatal(err)
				}
				defer sub.Cancel()
				rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/rollback/apply?token=secret", `{"id":"restore-proof"}`)
				wantStatus := http.StatusOK
				if denied {
					wantStatus = http.StatusForbidden
				}
				if rec.Code != wantStatus {
					t.Fatalf("HTTP=%d body=%s", rec.Code, rec.Body.String())
				}
				var correlation string
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
			wait:
				for {
					select {
					case e := <-sub.C:
						if e.Kind == event.KindOpCompleted || e.Kind == event.KindOpFailed {
							correlation = e.CorrelationID
							break wait
						}
					case <-timer.C:
						t.Fatal("missing restore operation terminal")
					}
				}
				var arc []event.Kind
				callID := ""
				encoded := base64.StdEncoding.EncodeToString([]byte("private snapshot proof bytes"))
				if err := k.Journal().Range(func(e *event.Event) error {
					if strings.Contains(string(e.Payload), "private snapshot proof bytes") || strings.Contains(string(e.Payload), encoded) {
						t.Errorf("snapshot content leaked into %s", e.Kind)
					}
					if e.CorrelationID != correlation {
						return nil
					}
					switch e.Kind {
					case event.KindOpInvoked, event.KindOpCompleted, event.KindOpFailed:
						var payload struct{ Op string }
						if err := json.Unmarshal(e.Payload, &payload); err != nil {
							return err
						}
						if payload.Op != controlplane.CmdFileRestore {
							t.Errorf("op=%q", payload.Op)
						}
					case event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult:
						var payload struct {
							Tool, Capability string
							CallID           string `json:"call_id"`
						}
						if err := json.Unmarshal(e.Payload, &payload); err != nil {
							return err
						}
						if callID == "" {
							callID = payload.CallID
						}
						if payload.Tool != controlplane.CmdFileRestore || payload.CallID == "" || payload.CallID != callID {
							t.Errorf("identity=%s", e.Payload)
						}
						if e.Kind == event.KindPolicyDecision && payload.Capability != string(capability) {
							t.Errorf("capability=%q", payload.Capability)
						}
					default:
						return nil
					}
					arc = append(arc, e.Kind)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				wantArc := []event.Kind{event.KindOpInvoked, event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult, event.KindOpCompleted}
				if denied {
					wantArc = []event.Kind{event.KindOpInvoked, event.KindPolicyDecision, event.KindToolResult, event.KindOpFailed}
				}
				if !reflect.DeepEqual(arc, wantArc) {
					t.Fatalf("arc=%v want=%v", arc, wantArc)
				}
				cat, err := loadRollbackCatalogAt(catalog)
				if err != nil {
					t.Fatal(err)
				}
				data, readErr := os.ReadFile(target)
				if denied {
					if readErr != nil || string(data) != "current bytes" || cat.Checkpoints[0].AppliedMS != 0 {
						t.Fatalf("denial changed file/catalog: bytes=%q error=%v catalog=%+v", data, readErr, cat)
					}
				} else {
					if cat.Checkpoints[0].AppliedMS <= 0 {
						t.Fatal("successful restore not marked applied")
					}
					if existed && (readErr != nil || string(data) != "private snapshot proof bytes") {
						t.Fatalf("content restore=%q error=%v", data, readErr)
					}
					if !existed && !os.IsNotExist(readErr) {
						t.Fatalf("absent restore retained file: %v", readErr)
					}
					if err := os.WriteFile(target, []byte("after restore"), 0o600); err != nil {
						t.Fatal(err)
					}
					repeat := httpJSON(t, s.Handler(), http.MethodPost, "/api/rollback/apply?token=secret", `{"id":"restore-proof"}`)
					if repeat.Code != http.StatusOK || !strings.Contains(repeat.Body.String(), `"applied":false`) {
						t.Fatalf("repeat=%d %s", repeat.Code, repeat.Body.String())
					}
					if data, err := os.ReadFile(target); err != nil || string(data) != "after restore" {
						t.Fatalf("repeat changed file: %q error=%v", data, err)
					}
				}
			})
		}
	}
}

func TestRollbackFileRestoreRequiresAvailableAudit(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := rollbackFileFixture(t, home, target, true)
	s, k := newFilesServerAt(t, home)
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/rollback/apply?token=secret", `{"id":"restore-proof"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("HTTP=%d %s", rec.Code, rec.Body.String())
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatalf("audit failure changed file: %q error=%v", data, err)
	}
	cat, err := loadRollbackCatalogAt(catalog)
	if err != nil || cat.Checkpoints[0].AppliedMS != 0 {
		t.Fatalf("audit failure changed catalog: %+v error=%v", cat, err)
	}
}
