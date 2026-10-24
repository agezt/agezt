// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/artifactapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The daemon's board store IS the contract's port: the board tool takes the
// store through the port, so the wiring site breaks instead of the tool if the
// module and the contract ever drift apart.
var _ boardapi.Store = (*board.Store)(nil)

// The daemon's artifact index IS the contract's port too: the registry hands
// the tools the index through the port, so the module's concrete index and
// the contract's port cannot drift apart unnoticed.
var _ artifactapi.Index = (*artifact.Index)(nil)

// TestBoardSpecBindsTheLiveStore: the board tool may not name the board
// module, so it takes the store as the contract's port — and the daemon binds
// the live one. A message the tool posts must land in the daemon's own store,
// not in a store of its own: two instances would silently clobber each other's
// last write.
func TestBoardSpecBindsTheLiveStore(t *testing.T) {
	st, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}

	spec := specBoard()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Late(built.Tool, toolreg.LateDeps{Board: st}); err != nil {
		t.Fatalf("Late: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"post","topic":"help","text":"please look"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the board tool errored: %s", res.Output)
	}
	msgs := st.Read("help", 10)
	if len(msgs) != 1 || msgs[0].Text != "please look" {
		t.Fatalf("the message did not reach the daemon's store: %+v", msgs)
	}
}

// TestArtifactsSpecBindsTheLiveIndex: the artifacts tool may not name the
// artifacts module, so it speaks the contract's rows — and the daemon binds
// the live index. An artifact the index holds must be listed by the tool.
func TestArtifactsSpecBindsTheLiveIndex(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	idx := k.ArtifactIndex()
	if idx == nil {
		t.Fatal("the kernel has no artifact index")
	}
	if _, err := idx.PutEntry(artifact.Entry{Kind: "image", Name: "cat.png", Mime: "image/png"}, []byte("PNGDATA"), 1); err != nil {
		t.Fatalf("PutEntry: %v", err)
	}

	spec := specArtifacts()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{Artifacts: idx}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the artifacts tool errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "cat.png") {
		t.Fatalf("the tool did not list the live artifact: %s", res.Output)
	}
}

// TestTheBoardPortStaysNilWhenUnwired: a daemon that could not open the board
// leaves the port NIL, and the board tool must then say the board is not
// available.
//
// The point is the difference between a nil port and a port wrapping a nil
// implementation: the port is an interface, so a nil *board.Store handed
// straight to it would be a NON-NIL interface, the tool would bind a store
// that is not there, and the first post would crash instead of reporting
// that the board is unavailable.
func TestTheBoardPortStaysNilWhenUnwired(t *testing.T) {
	spec := specBoard()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Late(built.Tool, toolreg.LateDeps{}); err != nil {
		t.Fatalf("Late: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"post","topic":"help","text":"please look"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("an unwired board must report that it is unavailable, got %+v", res)
	}
}

// TestTheArtifactPortStaysNilWhenUnwired: the same rule for the artifact
// index — a daemon with no index hands over a nil port, and the artifacts
// tool must report that the store is unavailable rather than bind nothing.
func TestTheArtifactPortStaysNilWhenUnwired(t *testing.T) {
	spec := specArtifacts()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unavailable") {
		t.Fatalf("an unwired index must report that it is unavailable, got %+v", res)
	}
}
