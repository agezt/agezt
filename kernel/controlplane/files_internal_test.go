// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestFileCommandsArePrimaryMutations(t *testing.T) {
	for _, command := range []string{CmdFileMkdir, CmdFileRename, CmdFileDelete, CmdFileRestore} {
		spec, ok := commandRegistry[command]
		if !ok || !spec.AppOwned || spec.ReadOnly || spec.TenantAllowed || spec.TenantRouted || spec.Streaming != StreamNone {
			t.Errorf("workspace command must be an audited primary-only mutation: %q %+v", command, spec)
		}
	}
}

// TestFileRestoreBindsTheServer: the restore reads the server's own catalog,
// runs the governed tool with the request's id as its call id under the
// operation's correlation, and keeps the checkpoint's member order.
func TestFileRestoreBindsTheServer(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), NewToolInvoker: apptools.NewInvoker})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	serverDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.txt")
	cp := rollbackstore.Checkpoint{ID: "c1", Kind: rollbackstore.KindFile, SubjectID: target, CreatedMS: 7, Before: map[string]any{"abs_path": target, "exists": true, "content_b64": base64.StdEncoding.EncodeToString([]byte("prior"))}}
	if err := rollbackstore.WriteAt(filepath.Join(serverDir, filepath.FromSlash(rollbackstore.RelativePath)), rollbackstore.Catalog{Checkpoints: []rollbackstore.Checkpoint{cp}}); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, serverDir)
	s.token = "primary"
	client, server := net.Pipe()
	defer client.Close()
	go s.handleConn(context.Background(), server)
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	raw, _ := json.Marshal(Request{ID: "req-77", Cmd: CmdFileRestore, Token: "primary", Args: map[string]any{"id": "c1"}})
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(client).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(line, []byte(`{"id":"req-77","type":"result","result":{"applied":true,"checkpoint":{"id":"c1","kind":"file.snapshot","action":"","subject_id":`)) || !bytes.Contains(line, []byte(`"created_ms":7,"applied_ms":`)) {
		t.Fatal(string(line))
	}
	var opCorr string
	calls := map[string]int{}
	_ = k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindOpInvoked:
			opCorr = e.CorrelationID
		case event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult:
			var p struct {
				CallID string `json:"call_id"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if e.CorrelationID != opCorr || opCorr == "" {
				t.Errorf("%s outside the operation's correlation", e.Kind)
			}
			calls[p.CallID]++
		}
		return nil
	})
	if calls["req-77"] != 3 || len(calls) != 1 {
		t.Fatal(calls)
	}
}
