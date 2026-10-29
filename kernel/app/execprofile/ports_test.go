// SPDX-License-Identifier: MIT

package execprofile

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// probeTool is the smallest tool a daemon can register.
type probeTool struct{}

func (probeTool) Definition() toolapi.ToolDef { return toolapi.ToolDef{Name: "shell"} }

func (probeTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{}, nil
}

// The execution profile's ports are filled by the APPLICATION from the kernel
// handle — the host hands over the ports, never the handle.
//
// The test pins the FILLED half: the inventory really READS the daemon. A port
// filled with nothing would report a host with no tools, and every profile
// that needs a tool would read as unsupported.
func TestTheExecutionProfilePortsReachTheDaemon(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"shell": probeTool{}},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Inventory == nil || ports.Policy == nil {
		t.Fatal("the port is not filled — the profile surface would report nothing")
	}

	inv := ports.Inventory()
	local, ok := inv.Find("local")
	if !ok {
		t.Fatalf("the inventory has no local profile: %+v", inv.Profiles)
	}
	if len(local.Tools) == 0 {
		t.Fatal("the inventory does not see the daemon's tools — the port did not read the daemon")
	}
	if !local.Routed {
		t.Fatal("the local profile is not routable — the daemon's tools did not reach the inventory")
	}
}

// A host with no daemon holds no tools and no warden.
//
// The port answers the EMPTY picture: the profiles that need a tool are
// unsupported, and the ones that do not are still there. That is the truth
// about a host with no daemon — and it is not the same answer as a daemon
// with no tools.
func TestTheExecutionProfilePortsAnswerAnEmptyHostWithoutADaemon(t *testing.T) {
	ports := PortsFromKernel(nil)
	if ports.Inventory == nil {
		t.Fatal("the port answers nothing for a host with no daemon — the surface would panic")
	}

	inv := ports.Inventory()
	local, ok := inv.Find("local")
	if !ok {
		t.Fatalf("the empty host has no local profile: %+v", inv.Profiles)
	}
	if len(local.Tools) != 0 {
		t.Fatalf("a host with no daemon has tools: %v", local.Tools)
	}
	if local.Routed {
		t.Fatal("a host with no daemon routes work")
	}
	if inv.HostOS == "" {
		t.Fatal("the empty host does not even know its own operating system")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own fields and the
// kernel's, so it does not depend on what the guard happens to own today.
func TestTheExecutionProfilePortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumField(); i++ {
		name := port.Field(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the execution profile's port field %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the execution profile's port %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
}
