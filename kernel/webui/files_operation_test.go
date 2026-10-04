// SPDX-License-Identifier: MIT

package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func newFilesServer(t *testing.T) (*Server, *runtime.Kernel) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	k, err := runtime.Open(runtime.Config{BaseDir: home, Provider: mock.New(), NewToolInvoker: apptools.NewInvoker})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	transport := controlplane.NewServer(k, home)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := transport.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { transport.Stop() })
	client, err := controlplane.NewClient(home)
	if err != nil {
		t.Fatal(err)
	}
	s := New(k.Bus(), client, "secret")
	s.SetAllowedHosts("example.com")
	s.SetPasswordStrict(false)
	s.allowQueryTokensForData = true
	return s, k
}

func TestFileMutationsHaveCorrelatedOperationAndToolAudit(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "allow", true: "deny"}[denied], func(t *testing.T) {
			root := t.TempDir()
			withFileRoot(t, root)
			for _, name := range []string{"rename-source", "delete-target"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			s, k := newFilesServer(t)
			if denied {
				k.Edict().SetLevel(edict.CapFileWrite, edict.LevelDeny)
				k.Edict().SetLevel(edict.CapFileDelete, edict.LevelDeny)
			}
			for _, tc := range []struct{ route, command, body string }{
				{"mkdir", controlplane.CmdFileMkdir, `{"path":"new-dir"}`},
				{"rename", controlplane.CmdFileRename, `{"from":"rename-source","to":"renamed"}`},
				{"delete", controlplane.CmdFileDelete, `{"path":"delete-target"}`},
			} {
				t.Run(tc.route, func(t *testing.T) {
					sub, err := k.Bus().Subscribe("op.>", 16)
					if err != nil {
						t.Fatal(err)
					}
					defer sub.Cancel()
					rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/files/"+tc.route+"?token=secret", tc.body)
					wantStatus := http.StatusOK
					if denied {
						wantStatus = http.StatusForbidden
					}
					if rec.Code != wantStatus {
						t.Fatalf("HTTP=%d body=%s", rec.Code, rec.Body.String())
					}
					var corr string
					timer := time.NewTimer(5 * time.Second)
					defer timer.Stop()
				wait:
					for {
						select {
						case e := <-sub.C:
							if e.Kind == event.KindOpCompleted || e.Kind == event.KindOpFailed {
								corr = e.CorrelationID
								break wait
							}
						case <-timer.C:
							t.Fatal("missing operation terminal audit")
						}
					}
					var arc []event.Kind
					var toolCallID string
					if err := k.Journal().Range(func(e *event.Event) error {
						if e.CorrelationID != corr {
							return nil
						}
						switch e.Kind {
						case event.KindOpInvoked, event.KindOpCompleted, event.KindOpFailed:
							var payload struct{ Op string }
							if err := json.Unmarshal(e.Payload, &payload); err != nil {
								return err
							}
							if payload.Op != tc.command {
								t.Errorf("op identity=%q", payload.Op)
							}
						case event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult:
							var payload struct {
								Tool, Capability string
								CallID           string `json:"call_id"`
							}
							if err := json.Unmarshal(e.Payload, &payload); err != nil {
								return err
							}
							if toolCallID == "" {
								toolCallID = payload.CallID
							}
							if payload.Tool != tc.command || payload.CallID == "" || payload.CallID != toolCallID {
								t.Errorf("tool identity=%s", e.Payload)
							}
							wantCapability := "file.write"
							if tc.command == controlplane.CmdFileDelete {
								wantCapability = "file.delete"
							}
							if e.Kind == event.KindPolicyDecision && payload.Capability != wantCapability {
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
					want := []event.Kind{event.KindOpInvoked, event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult, event.KindOpCompleted}
					if denied {
						want = []event.Kind{event.KindOpInvoked, event.KindPolicyDecision, event.KindToolResult, event.KindOpFailed}
					}
					if !reflect.DeepEqual(arc, want) {
						t.Fatalf("audit=%v want=%v", arc, want)
					}
				})
			}
			_, mkdirErr := os.Stat(filepath.Join(root, "new-dir"))
			_, sourceErr := os.Stat(filepath.Join(root, "rename-source"))
			_, destinationErr := os.Stat(filepath.Join(root, "renamed"))
			_, deleteErr := os.Stat(filepath.Join(root, "delete-target"))
			if denied {
				if !os.IsNotExist(mkdirErr) || sourceErr != nil || !os.IsNotExist(destinationErr) || deleteErr != nil {
					t.Fatalf("denial changed disk: mkdir=%v source=%v destination=%v delete=%v", mkdirErr, sourceErr, destinationErr, deleteErr)
				}
			} else if mkdirErr != nil || !os.IsNotExist(sourceErr) || destinationErr != nil || !os.IsNotExist(deleteErr) {
				t.Fatalf("allowed effects missing: mkdir=%v source=%v destination=%v delete=%v", mkdirErr, sourceErr, destinationErr, deleteErr)
			}
		})
	}
}

func TestFileMutationDenialDoesNotBootstrapRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	withFileRoot(t, root)
	s, k := newFilesServer(t)
	k.Edict().SetLevel(edict.CapFileWrite, edict.LevelDeny)
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/files/mkdir?token=secret", `{"path":"nested","parents":true}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("HTTP=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("denied mutation created workspace root: %v", err)
	}
}

func TestFileDeleteHonorsItsOwnCapability(t *testing.T) {
	root := t.TempDir()
	withFileRoot(t, root)
	target := filepath.Join(root, "keep")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, k := newFilesServer(t)
	k.Edict().SetLevel(edict.CapFileDelete, edict.LevelDeny)
	for _, tc := range []struct {
		route, body string
		status      int
	}{
		{"mkdir", `{"path":"allowed"}`, http.StatusOK},
		{"delete", `{"path":"keep"}`, http.StatusForbidden},
	} {
		rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/files/"+tc.route+"?token=secret", tc.body)
		if rec.Code != tc.status {
			t.Fatalf("%s HTTP=%d body=%s", tc.route, rec.Code, rec.Body.String())
		}
	}
	if content, err := os.ReadFile(target); err != nil || string(content) != "keep" {
		t.Fatalf("delete denial changed target: %q error=%v", content, err)
	}
}

func TestFileMutationRequiresAvailablePolicyAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	withFileRoot(t, root)
	s, k := newFilesServer(t)
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/files/mkdir?token=secret", `{"path":"nested","parents":true}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("HTTP=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("unavailable audit allowed filesystem effects: %v", err)
	}
}
