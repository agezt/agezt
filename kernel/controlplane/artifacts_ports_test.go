// SPDX-License-Identifier: MIT
package controlplane

import (
	"path/filepath"
	"testing"

	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	blobs "github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the artifacts' ports — the APPLICATION fills them
// from the kernel handle.
//
// The test pins that binding end to end: the daemon's own artifact store
// answers for the daemon's kernel, and a kernel opened without one holds none.
func TestTheAdapterBindsTheArtifactPortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	store, err := blobs.Open(filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatalf("open the artifact store: %v", err)
	}
	index, err := blobs.OpenIndex(store, filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatalf("open the artifact index: %v", err)
	}
	root, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Artifacts: store, ArtifactIndex: index})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(root, t.TempDir())

	// The daemon's own ports answer for its own kernel.
	rootPorts := appartifacts.PortsFromKernel(root)
	if got := s.artifactPorts().Store(); got != rootPorts.Store() {
		t.Fatal("the daemon's artifact ports do not answer for the daemon's own kernel")
	}
	if got := s.artifactPorts().Index(); got != rootPorts.Index() {
		t.Fatal("the daemon's artifact ports do not answer for the daemon's own kernel")
	}
}

// A daemon opened WITHOUT an artifact store holds none, and the binding says
// so instead of handing the service something.
//
// The refusal belongs to the service: the port hands over nothing, and the
// service answers with its own refusal.
func TestTheAdapterBindsTheArtifactPortsToAHostWithNoStore(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(root, t.TempDir())
	if got := s.artifactPorts().Store(); got != nil {
		t.Fatal("a daemon with no artifact store hands one over")
	}
	if got := s.artifactPorts().Index(); got != nil {
		t.Fatal("a daemon with no artifact index hands one over")
	}
}
