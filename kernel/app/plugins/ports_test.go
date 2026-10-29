// SPDX-License-Identifier: MIT
package plugins_test

import (
	"reflect"
	"testing"

	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The plugins family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the manifest the port reports is the
// daemon's own manifest. A port bound to nothing would report no plugins at
// all, and the operator would read "no external plugins" for a daemon that has
// them.
func TestThePluginPortsReachTheKernelManifest(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(),
		Plugins: []runtime.PluginInfo{{Prefix: "acme", Path: "/opt/acme"}}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appplugins.PortsFromKernel(k)

	manifest := ports.Manifest()
	if len(manifest) != 1 {
		t.Fatalf("the plugin port reports %d plugins, want the daemon's one", len(manifest))
	}
	if manifest[0].Prefix != "acme" {
		t.Fatalf("the plugin port reports the prefix %q", manifest[0].Prefix)
	}
	if len(ports.Manifest()) != len(k.Plugins()) {
		t.Fatal("the plugin port reports a different manifest than the kernel holds")
	}
}

// A host with no kernel has no manifest.
//
// The port answers for that case with no plugins: the daemon that was given
// none holds none, and the empty manifest is what the operator should read.
func TestThePluginPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := appplugins.PortsFromKernel(nil)

	if got := ports.Manifest(); len(got) != 0 {
		t.Fatalf("a host with no kernel reports %d plugins", len(got))
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestThePluginPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(appplugins.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the plugin port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the plugin port has no methods — the family cannot answer anything")
	}
}
