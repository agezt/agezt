// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/plugins/tools/codeexec"
)

// TestTheCodeExecSpecInstallsTheExecutorThroughTheRow: the code_exec tool
// installs itself as the Verifier's executor through the registry's row
// (W3.39g) — a plain closure, because a function value names no type.
//
// The row must receive the tool: a Verifier without an executor always
// critiques, and the agent's code would never actually run.
func TestTheCodeExecSpecInstallsTheExecutorThroughTheRow(t *testing.T) {
	var got toolregapi.ScriptRunner
	installExecutor(&codeexec.Tool{}, toolregapi.KernelDeps{
		ConductorExec: func(x toolregapi.ScriptRunner) { got = x },
	})
	if got == nil {
		t.Fatal("the row must receive the tool")
	}
}

// TestTheCodeExecSpecRefusesANilExecutor: a nil tool must never reach the
// row.
//
// A nil *codeexec.Tool handed to an interface parameter is a NON-NIL
// interface: the Verifier would believe it can execute code and then call
// into a tool that is not there.
func TestTheCodeExecSpecRefusesANilExecutor(t *testing.T) {
	called := false
	installExecutor(nil, toolregapi.KernelDeps{
		ConductorExec: func(toolregapi.ScriptRunner) { called = true },
	})
	if called {
		t.Fatal("a nil tool must not reach the row")
	}
}

// TestTheForgeSpecSurvivesAMissingRunnerPort: the tool_forge tool drives the
// forge's lifecycle through the CONTRACT's port (W3.39g) — the registry's
// row is no longer the live kernel, because a plugin may not name a module's
// type.
//
// A daemon that hands over no runner must leave the tool reporting that the
// forge is unavailable — not dereferencing nil.
func TestTheForgeSpecSurvivesAMissingRunnerPort(t *testing.T) {
	spec := specToolForge()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"draft","name":"echo","description":"echoes its input","language":"python","code":"print(1)"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired forge runner must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not available") {
		t.Fatalf("the unwired tool must say it is not available: %s", res.Output)
	}
}

// TestTheSendMediaSpecResolvesArtifactsThroughTheRow: the send_media tool
// resolves an artifact ref through the registry's row (W3.39g) — a plain
// closure, because a function value names no type.
//
// The bytes the row hands back must arrive at the channel: a row that
// resolved to nothing would attach an empty file to the operator's chat
// while reporting a success.
func TestTheSendMediaSpecResolvesArtifactsThroughTheRow(t *testing.T) {
	spec := specSendMedia()
	built, err := spec.Build(toolregapi.BuildDeps{
		NotifyTargets: map[string][]string{"slack": {"C1"}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var sent []channel.Attachment
	err = spec.Late(built.Tool, toolregapi.LateDeps{
		ChannelSendMedia: func(_ context.Context, _, _, _ string, atts []channel.Attachment) error {
			sent = atts
			return nil
		},
		ArtifactBytes: func(ref string) ([]byte, error) {
			if ref != "ref-1" {
				return nil, fmt.Errorf("unexpected ref %q", ref)
			}
			return []byte("payload"), nil
		},
	})
	if err != nil {
		t.Fatalf("Late: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"artifact":"ref-1","kind":"file"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the tool errored: %s", res.Output)
	}
	if len(sent) != 1 {
		t.Fatalf("the channel must receive exactly one attachment, got %d", len(sent))
	}
	if string(sent[0].Data) != "payload" {
		t.Fatalf("the attachment must carry the resolved bytes, got %q", sent[0].Data)
	}
}

// TestTheSendMediaSpecSurvivesAMissingArtifactRow: a daemon that hands over
// no artifact row must leave the tool saying that the store is unavailable —
// not attaching an empty file, and not dereferencing nil.
func TestTheSendMediaSpecSurvivesAMissingArtifactRow(t *testing.T) {
	spec := specSendMedia()
	built, err := spec.Build(toolregapi.BuildDeps{
		NotifyTargets: map[string][]string{"slack": {"C1"}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var sent []channel.Attachment
	err = spec.Late(built.Tool, toolregapi.LateDeps{
		ChannelSendMedia: func(_ context.Context, _, _, _ string, atts []channel.Attachment) error {
			sent = atts
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Late: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"artifact":"ref-1","kind":"file"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired artifact row must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "artifact store unavailable") {
		t.Fatalf("the unwired tool must say the store is unavailable: %s", res.Output)
	}
	if len(sent) != 0 {
		t.Fatalf("nothing may be delivered without the bytes, got %d attachments", len(sent))
	}
}
