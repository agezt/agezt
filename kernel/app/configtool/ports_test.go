// SPDX-License-Identifier: MIT

package configtool

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The config tool's ports are filled by the APPLICATION from the kernel
// handle — the composition root hands over the tool's ports, never the
// handle.
//
// The test pins the FILLED half: the port really reaches the daemon. A port
// bound to nothing would answer "no daemon" for every question, and the
// tool would refuse to do its work while looking perfectly wired.
func TestTheConfigToolPortsReachTheDaemon(t *testing.T) {
	base := t.TempDir()
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:  base,
		Provider: mock.New(),
		// The catalog directory is the DAEMON's file (W3.51): the test opens
		// it and hands the kernel the contract's store port.
		CatalogStore: catalog.NewStore(filepath.Join(base, "catalog")),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddProfile(agentapi.Profile{Slug: "researcher"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	ports := PortsFromKernel(k)
	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the tool would refuse to do its work")
	}

	got, found, err := ports.EditProfile("researcher", func(p *agentapi.Profile) {
		p.ConfigOverrides = map[string]string{"AGEZT_MODEL": "agent-model"}
	})
	if err != nil {
		t.Fatalf("EditProfile: %v", err)
	}
	if !found {
		t.Fatal("the port must find the profile the roster holds")
	}
	if got.ConfigOverrides["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the edit is not visible on the answer: %+v", got.ConfigOverrides)
	}
	stored, ok := k.Roster().Get("researcher")
	if !ok || stored.ConfigOverrides["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the edit was not stored in the roster: %+v ok=%v", stored.ConfigOverrides, ok)
	}

	if err := ports.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
}

// A host with no daemon holds no daemon to answer for.
//
// The port says so ONCE — and the tool is handed a nil port rather than a
// port that calls into nothing.
func TestTheConfigToolPortsRefuseForAHostWithNoDaemon(t *testing.T) {
	ports := PortsFromKernel(nil)

	if ports.Ready() {
		t.Fatal("a host with no daemon reports a daemon")
	}
	if _, found, err := ports.EditProfile("researcher", func(*agentapi.Profile) {}); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the edit answered %v, want %v", err, ErrNoDaemon)
	} else if found {
		t.Error("the edit found an agent on a host with no daemon")
	}
	if err := ports.Rebuild(); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the rebuild answered %v, want %v", err, ErrNoDaemon)
	}
	if got := NewKernelSource(ports); got != nil {
		t.Fatalf("a host with no daemon hands the tool a port: %v", got)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheConfigToolPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&kernelruntime.Kernel{})
	if port.NumMethod() == 0 {
		t.Fatal("the config tool's port has no methods — the tool cannot ask anything")
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the config tool's port %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	for i := 0; i < port.NumField(); i++ {
		name := port.Field(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the config tool's port field %s is the kernel's own verb — a port field may not carry the name of the verb it stands for", name)
		}
	}
}
