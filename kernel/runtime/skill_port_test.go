// SPDX-License-Identifier: MIT

package runtime_test

// Provenance: the kernel's contract tests for the skill curator (W3.55) —
//             that the kernel's face is the contract's port, that a kernel
//             without a skill library holds a NIL port, and that the port
//             answers the LIVE rows.

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheSkillFaceIsTheContractsPort: the kernel's own face is the contract's
// port.
//
// The width is checked STATICALLY, on the accessor itself: a narrowed face
// would still answer the read side, so only the declared return type can tell
// the two apart.
func TestTheSkillFaceIsTheContractsPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got, want := reflect.TypeOf(k.Forge).Out(0), reflect.TypeOf((*skillapi.Curator)(nil)).Elem(); got != want {
		t.Fatalf("the kernel's skill face is %v, want the contract's curator port %v", got, want)
	}
}

// TestTheSkillPortStaysNilWithoutALibrary: a kernel opened without a skill
// library hands over a NIL port — not a port wrapping nothing.
//
// The check is on the CONCRETE value: a nil curator handed to an interface
// parameter is already a non-nil interface, and the first write would then
// call into a curator that is not there.
func TestTheSkillPortStaysNilWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Forge(); got != nil {
		t.Fatalf("a kernel with no skill library must yield a nil curator port, got %v", got)
	}
}

// TestTheSkillPortAnswersTheLiveLibrary: the port is not a shape — it answers
// the LIVE rows. A skill written through the port has to be in the library the
// host opened.
func TestTheSkillPortAnswersTheLiveLibrary(t *testing.T) {
	k, err := runtime.Open(withSkill(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.Forge() == nil {
		t.Fatal("the kernel holds no skill port")
	}
	sk, created, err := k.Forge().Create("seed", skillapi.CreateSpec{Name: "diagnose-ci", Body: "steps"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created {
		t.Fatal("the skill must be newly created")
	}
	got, found, err := k.Forge().Get(sk.ID)
	if err != nil || !found {
		t.Fatalf("the port must answer the live rows, got %+v (%v, %v)", got, found, err)
	}
	if got.Name != "diagnose-ci" {
		t.Fatalf("the port answered %q, want diagnose-ci", got.Name)
	}
	all, err := k.Forge().List()
	if err != nil || len(all) != 1 {
		t.Fatalf("the port must answer the live skills, got %d (%v)", len(all), err)
	}
}
