// SPDX-License-Identifier: MIT

package workboardapi

import (
	"reflect"
	"testing"
	"time"
)

// TestTheWorkboardRunnerPortHasExactlyThirteenMethods: the workboard port is
// the whole of what the workboard tool may do to the daemon's board —
// create, claim, heartbeat, comment, block, fail, unblock, complete,
// archive, link, set the retry policy, add a dependency and reclaim a stale
// task. A fourteenth method would be a new reach into the daemon, and it
// would have to be earned here.
//
// Thirteen is a wide port, and that is worth naming: the workboard tool is a
// first-class actor on the board (it creates, claims, moves and links tasks),
// so its reach IS the board's whole surface. It is still one port rather
// than thirteen, because the daemon has one board — and every method here
// names the contract's own rows.
func TestTheWorkboardRunnerPortHasExactlyThirteenMethods(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Runner must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 13 {
		t.Fatalf("the port has %d methods, want 13", typ.NumMethod())
	}
	for _, name := range []string{
		"CreateWorkboardTask", "ClaimWorkboardTask", "HeartbeatWorkboardTask",
		"CommentWorkboardTask", "BlockWorkboardTask", "FailWorkboardTask",
		"UnblockWorkboardTask", "CompleteWorkboardTask", "ArchiveWorkboardTask",
		"LinkWorkboardTask", "SetWorkboardRetryPolicy",
		"AddWorkboardDependency", "ReclaimStaleWorkboardTask",
	} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheWorkboardRunnerNamesOnlyContractRows: every type the port names must
// be a contract row, a builtin or a standard-library type. A module's type in
// a port's signature would drag that module into every plugin that holds
// the port — the very leak the contract layer exists to stop.
func TestTheWorkboardRunnerNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	task := reflect.TypeOf(Task{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")

	create, _ := typ.MethodByName("CreateWorkboardTask")
	// CreateWorkboardTask(corr string, spec CreateSpec) (Task, bool, error)
	if create.Type.NumIn() != 2 || create.Type.In(0) != str || create.Type.In(1) != reflect.TypeOf(CreateSpec{}) {
		t.Errorf("CreateWorkboardTask must take (string, CreateSpec), got %v", create.Type)
	}
	if create.Type.NumOut() != 3 || create.Type.Out(0) != task || create.Type.Out(1) != reflect.TypeOf(false) || create.Type.Out(2) != errType {
		t.Errorf("CreateWorkboardTask must answer (Task, bool, error), got %v", create.Type)
	}

	fail, _ := typ.MethodByName("FailWorkboardTask")
	// FailWorkboardTask(corr, id, actor, reason string) (Task, RetryDecision, error)
	if fail.Type.NumIn() != 4 || fail.Type.In(3) != str {
		t.Errorf("FailWorkboardTask must take four strings, got %v", fail.Type)
	}
	if fail.Type.NumOut() != 3 || fail.Type.Out(1) != reflect.TypeOf(RetryDecision{}) || fail.Type.Out(2) != errType {
		t.Errorf("FailWorkboardTask must answer (Task, RetryDecision, error), got %v", fail.Type)
	}

	policy, _ := typ.MethodByName("SetWorkboardRetryPolicy")
	// SetWorkboardRetryPolicy(corr, id, actor string, policy *RetryPolicy) (Task, error)
	if policy.Type.NumIn() != 4 || policy.Type.In(3) != reflect.TypeOf((*RetryPolicy)(nil)) {
		t.Errorf("SetWorkboardRetryPolicy must take the contract's policy pointer, got %v", policy.Type)
	}

	reclaim, _ := typ.MethodByName("ReclaimStaleWorkboardTask")
	// ReclaimStaleWorkboardTask(corr, id, actor string, staleAfter time.Duration) (Task, error)
	if reclaim.Type.NumIn() != 4 || reclaim.Type.In(3) != reflect.TypeOf(time.Duration(0)) {
		t.Errorf("ReclaimStaleWorkboardTask must take a time.Duration, got %v", reclaim.Type)
	}
	if reclaim.Type.NumOut() != 2 || reclaim.Type.Out(0) != task || reclaim.Type.Out(1) != errType {
		t.Errorf("ReclaimStaleWorkboardTask must answer (Task, error), got %v", reclaim.Type)
	}
}
