// SPDX-License-Identifier: MIT

package artifacts_test

import (
	"path/filepath"
	"reflect"
	"testing"

	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	blobs "github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The artifacts family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the store the port hands over is the
// daemon's own store, and the index is the daemon's own index. A port bound to
// nothing would hand the service no store at all, and the service would refuse
// every read.
func TestTheArtifactPortsReachTheKernelStore(t *testing.T) {
	dir := t.TempDir()
	store, err := blobs.Open(filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatalf("open the artifact store: %v", err)
	}
	index, err := blobs.OpenIndex(store, filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatalf("open the artifact index: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Artifacts: store, ArtifactIndex: index})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appartifacts.PortsFromKernel(k)

	if ports.Store() == nil {
		t.Fatal("the artifact port hands over no store — the daemon holds one")
	}
	if ports.Index() == nil {
		t.Fatal("the artifact port hands over no index — the daemon holds one")
	}
	if ports.Store() != k.Artifacts() {
		t.Fatal("the artifact port hands over a different store than the kernel holds")
	}
	if ports.Index() != k.ArtifactIndex() {
		t.Fatal("the artifact port hands over a different index than the kernel holds")
	}
}

// A host with no kernel has no artifact store and no index.
//
// The port answers for that case with nothing at all: the store and the index
// MAY be absent, and the surfaces that read them refuse instead of reading
// from nothing.
func TestTheArtifactPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := appartifacts.PortsFromKernel(nil)

	if ports.Store() != nil {
		t.Fatal("a host with no kernel hands over an artifact store")
	}
	if ports.Index() != nil {
		t.Fatal("a host with no kernel hands over an artifact index")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheArtifactPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(appartifacts.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the artifact port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the artifact port has no methods — the family cannot answer anything")
	}
}
