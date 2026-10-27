// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/cadence/systemtasks"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// TestTheSystemTasksRunOnTheContractsPorts: the daemon hands the system
// tasks the CONTRACT's port set — and not the kernel.
//
// The check is STATICALLY on the signature: a system task that took the
// kernel again would compile, and only the declared parameter type can tell
// the two apart. This is the edge the slice closes, so the shape of the
// dispatch is what has to be pinned.
func TestTheSystemTasksRunOnTheContractsPorts(t *testing.T) {
	typ := reflect.TypeOf(systemtasks.Run)
	if typ.NumIn() != 5 {
		t.Fatalf("Run takes %d arguments, want 5: %v", typ.NumIn(), typ)
	}
	if got, want := typ.In(1), reflect.TypeOf(cadenceapi.SystemTaskPorts{}); got != want {
		t.Fatalf("the system tasks run on %v, want the contract's port set %v", got, want)
	}
}

// TestTheKernelHandsOverTheContractsPortSet: the kernel's own face is the
// contract's port set.
//
// The check is on the DECLARED return type: a narrowed face would still
// answer the tasks, so only the declared type can tell the two apart.
func TestTheKernelHandsOverTheContractsPortSet(t *testing.T) {
	typ := reflect.TypeOf((*kernelruntime.Kernel).SystemTaskPorts)
	if typ.NumOut() != 1 {
		t.Fatalf("SystemTaskPorts answers %d values, want 1: %v", typ.NumOut(), typ)
	}
	if got, want := typ.Out(0), reflect.TypeOf(cadenceapi.SystemTaskPorts{}); got != want {
		t.Fatalf("the kernel hands over %v, want the contract's port set %v", got, want)
	}
}
