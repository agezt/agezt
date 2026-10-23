// SPDX-License-Identifier: MIT

package config

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/configapi"
)

const (
	contractPkg = "github.com/agezt/agezt/kernel/contract/configapi"
	runtimePkg  = "github.com/agezt/agezt/kernel/runtime"
)

// TestTheToolNamesTheContractPort: the config tool is a PLUGIN, so it may
// reach the contract layer and nothing else. The live daemon arrives as the
// contract's port — the tool must not hold the daemon's implementation, and
// it must not name the runtime.
func TestTheToolNamesTheContractPort(t *testing.T) {
	field, ok := reflect.TypeOf(Tool{}).FieldByName("kernel")
	if !ok {
		t.Fatal("the tool must keep the port it writes through")
	}
	if field.Type != reflect.TypeOf((*configapi.Kernel)(nil)).Elem() {
		t.Fatalf("the tool's port must be the contract's, got %v", field.Type)
	}
	if field.Type.PkgPath() != contractPkg {
		t.Fatalf("the tool's port must come from %s, got %s", contractPkg, field.Type.PkgPath())
	}
	if field.Type.PkgPath() == runtimePkg {
		t.Fatalf("the tool must not name the runtime %s", runtimePkg)
	}
}

// TestTheSetKernelTakesThePort: the binding the daemon uses is the contract's
// port, so a plugin can never be handed the kernel itself.
func TestTheSetKernelTakesThePort(t *testing.T) {
	typ, ok := reflect.TypeOf(&Tool{}).MethodByName("SetKernel")
	if !ok {
		t.Fatal("the tool must have a SetKernel method")
	}
	if typ.Type.NumIn() != 2 {
		t.Fatalf("SetKernel must take the port, got %v", typ.Type)
	}
	if typ.Type.In(1) != reflect.TypeOf((*configapi.Kernel)(nil)).Elem() {
		t.Fatalf("SetKernel must take the contract's port, got %v", typ.Type.In(1))
	}
}
