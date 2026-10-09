// SPDX-License-Identifier: MIT
package controlplane

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func artifactAppFixture(t *testing.T) (*runtime.Kernel, *Server, artifact.Entry, *mock.Provider) {
	t.Helper()
	dir := t.TempDir()
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	entry, err := k.ArtifactIndex().PutEntry(artifact.Entry{Name: "owned", Kind: "file", Source: "run", Corr: "owned"}, []byte("owned"), 100)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	return k, s, entry, provider
}
func TestArtifactAppSocketRequiresAuditBeforeDeleteAndCollect(t *testing.T) {
	for _, item := range []struct {
		cmd string
		dry bool
	}{{CmdArtifactDelete, false}, {CmdArtifactCollect, false}, {CmdArtifactCollect, true}} {
		t.Run(item.cmd+map[bool]string{false: "/real", true: "/dry"}[item.dry], func(t *testing.T) {
			k, s, entry, provider := artifactAppFixture(t)
			before := k.ArtifactIndex().List(artifact.Filter{})
			blob, err := k.Artifacts().Get(entry.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if err := k.Journal().Close(); err != nil {
				t.Fatal(err)
			}
			response := callAppHost(t, s, Request{ID: item.cmd, Cmd: item.cmd, Token: "primary", Args: map[string]any{"id": entry.ID, "dry_run": item.dry}})[0]
			after := k.ArtifactIndex().List(artifact.Filter{})
			bytes, err := k.Artifacts().Get(entry.Ref)
			if response.Type != RespError || !strings.Contains(response.Error, "journal") || !reflect.DeepEqual(before, after) || err != nil || !reflect.DeepEqual(blob, bytes) || provider.CallCount() != 0 {
				t.Fatalf("EXPECTED: audit unavailable => no metadata/blob/provider effect; ACTUAL: cmd=%s response=%+v changed=%v blobErr=%v", item.cmd, response, !reflect.DeepEqual(before, after), err)
			}
		})
	}
}
func TestStorageArtifactMetadataComesFromSixAppSpecs(t *testing.T) {
	if len(storageOperations) != 2 || len(artifactOperations) != 4 {
		t.Fatal(len(storageOperations), len(artifactOperations))
	}
	reads := map[string]bool{CmdStorageStats: true, CmdDiskStats: true, CmdArtifactGet: true, CmdArtifactList: true, CmdArtifactDelete: false, CmdArtifactCollect: false}
	seen := map[string]bool{}
	for _, operation := range append(storageOperations, artifactOperations...) {
		spec := operation.Spec()
		wire, exists := commandRegistry[spec.Name]
		readOnly, known := reads[spec.Name]
		if !known || !exists || seen[spec.Name] || !wire.AppOwned || wire.ReadOnly != readOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.ReadOnly != readOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input == nil || spec.Output == nil || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
}
func TestStorageArtifactAppReadsRetainUnauditedUnaryAvailability(t *testing.T) {
	k, s, entry, _ := artifactAppFixture(t)
	for _, cmd := range []string{CmdStorageStats, CmdDiskStats, CmdArtifactGet, CmdArtifactList} {
		responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"ref": entry.Ref, "kind": "file", "unused": true}})
		if len(responses) != 1 || responses[0].Type != RespResult {
			t.Fatal(cmd, responses)
		}
	}
	count := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked && (strings.HasPrefix(e.Subject, "op.artifact_") || e.Subject == "op.storage_stats" || e.Subject == "op.disk_stats") {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("read audit count", count)
	}
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{CmdStorageStats, CmdDiskStats, CmdArtifactGet, CmdArtifactList} {
		response := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"ref": entry.Ref}})[0]
		if response.Type != RespResult {
			t.Fatal(cmd, response)
		}
	}
}
func TestArtifactAppAdmissionPreservesPresenceStrictStringsAndDryRunForms(t *testing.T) {
	k, s, entry, _ := artifactAppFixture(t)
	for _, item := range []struct{ cmd, key, want string }{{CmdArtifactGet, "ref", "args.ref"}, {CmdArtifactDelete, "id", "args.id"}, {CmdArtifactList, "kind", "args.kind"}, {CmdArtifactCollect, "dry_run", "args.dry_run"}} {
		for _, raw := range []any{nil, float64(1)} {
			response := callAppHost(t, s, Request{ID: item.cmd, Cmd: item.cmd, Token: "primary", Args: map[string]any{item.key: raw}})[0]
			want := item.want + " must be a string"
			if item.key == "dry_run" {
				want = item.want + " must be a boolean"
			}
			if response.Type != RespError || response.Error != want {
				t.Fatal(item, raw, response)
			}
		}
	}
	for _, raw := range []any{true, "true", "1", "FALSE", " false "} {
		response := callAppHost(t, s, Request{ID: "dry", Cmd: CmdArtifactCollect, Token: "primary", Args: map[string]any{"dry_run": raw, "older_than_days": "7"}})[0]
		if response.Type != RespResult || response.Result["dry_run"] != true || response.Result["older_than_days"] != float64(7) || response.Result["count"] != float64(1) {
			t.Fatal(raw, response)
		}
	}
	response := callAppHost(t, s, Request{ID: "get", Cmd: CmdArtifactGet, Token: "primary", Args: map[string]any{"ref": " " + entry.Ref}})[0]
	if response.Type != RespError || !strings.Contains(response.Error, "malformed") {
		t.Fatal("raw strict ref was trimmed", response)
	}
	if k.ArtifactIndex().Count() != 1 {
		t.Fatal("dry-run changed index")
	}
	raw, _ := json.Marshal(callAppHost(t, s, Request{ID: "list", Cmd: CmdArtifactList, Token: "primary"})[0].Result)
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	record := values["entries"].([]any)[0].(map[string]any)
	for _, key := range []string{"mime", "sender", "caption"} {
		if _, exists := record[key]; !exists {
			t.Fatal("required empty metadata lost", key)
		}
	}
	response = callAppHost(t, s, Request{ID: "collect", Cmd: CmdArtifactCollect, Token: "primary", Args: map[string]any{"dry_run": "0"}})[0]
	if response.Type != RespResult || response.Result["dry_run"] != false || response.Result["count"] != float64(1) || k.ArtifactIndex().Count() != 0 {
		t.Fatal(response)
	}
	if _, present := response.Result["candidates"]; present {
		t.Fatal("real collection returned candidates")
	}
}
func TestArtifactAppMutationsHaveOneOwnedAuditSpan(t *testing.T) {
	for _, cmd := range []string{CmdArtifactDelete, CmdArtifactCollect} {
		t.Run(cmd, func(t *testing.T) {
			k, s, entry, _ := artifactAppFixture(t)
			response := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"id": entry.ID, "dry_run": false}})[0]
			if response.Type != RespResult {
				t.Fatal(response)
			}
			rows := []*event.Event{}
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Subject == "op."+cmd {
					copy := *e
					rows = append(rows, &copy)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 || rows[0].Kind != event.KindOpInvoked || rows[1].Kind != event.KindOpCompleted || rows[0].CorrelationID == "" || rows[0].CorrelationID != rows[1].CorrelationID || k.ArtifactIndex().Count() != 0 {
				t.Fatal(rows, k.ArtifactIndex().Count())
			}
		})
	}
}
