// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
)

// The COUNCIL's verbs, which the APPLICATION owns. The council's panel, its
// seating, its convening and its conductor are the application's business: the
// host hands over the kernel handle, and the application decides what the
// council reads and how it answers.
//
// The gate cannot hold this — it counts packages, not verbs — so the test
// does: the host must not call these on the handle itself.
var councilOwnedVerbs = []string{"Council", "CouncilDefaultMembers", "SetCouncilMembers", "Conduct"}

// rosterAccessor finds the kernel's roster verbs in the file that declares
// them.
var rosterAccessor = regexp.MustCompile(`func \(k \*Kernel\) ([A-Za-z_][A-Za-z0-9_]*)\(`)

// rosterAccessorFile is where the kernel's roster verbs live.
const rosterAccessorFile = "../runtime/accessors_roster.go"

// rosterOwnedVerbs are the kernel's ROSTER verbs: who an agent is, who all
// the agents are, adding one, editing one, removing one, pausing one, retiring
// one and what depends on one.
//
// The list is READ from the kernel's own roster accessor file rather than
// written here: a roster verb added to the kernel is guarded the day it
// lands, and a verb dropped from the kernel leaves the guard on its own. A
// hand-written list would quietly stop guarding the verbs nobody remembered
// to add.
func rosterOwnedVerbs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(rosterAccessorFile)
	if err != nil {
		t.Fatalf("read the kernel's roster accessors: %v", err)
	}
	matches := rosterAccessor.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("no roster verbs found — the kernel's roster accessors moved?")
	}
	verbs := make([]string, 0, len(matches))
	for _, m := range matches {
		verbs = append(verbs, m[1])
	}
	return verbs
}

// applicationOwnedVerbs are all the verbs the application owns.
func applicationOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return append(append([]string{}, councilOwnedVerbs...), rosterOwnedVerbs(t)...)
}

// The guard's list must hold verbs the kernel actually has: a verb nobody has
// guards nothing, and it reads like it guards something.
func TestTheGuardNamesOnlyVerbsTheKernelHas(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for _, verb := range applicationOwnedVerbs(t) {
		if _, ok := kernel.MethodByName(verb); !ok {
			t.Errorf("the guard names %s — the kernel has no such verb", verb)
		}
	}
}

// And it must hold EVERY roster verb the kernel has: the roster's business
// belongs to the application, and the list that says so is derived from the
// kernel itself.
//
// The verbs are read HERE, not through the helper the guard uses: a helper
// that quietly drops a verb must not be able to hide the drop from the check
// that is supposed to catch it.
func TestTheGuardNamesEveryRosterVerbTheKernelHas(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "runtime", "accessors_roster.go"))
	if err != nil {
		t.Fatalf("read the kernel's roster accessors: %v", err)
	}
	declared := regexp.MustCompile(`func \(k \*Kernel\) ([A-Za-z_][A-Za-z0-9_]*)\(`).FindAllStringSubmatch(string(raw), -1)
	if len(declared) == 0 {
		t.Fatal("no roster verbs found — the kernel's roster accessors moved?")
	}
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for _, m := range declared {
		verb := m[1]
		if _, ok := kernel.MethodByName(verb); !ok {
			t.Errorf("%s is declared in the roster accessor file but the kernel has no such method", verb)
		}
		if !guarded[verb] {
			t.Errorf("the kernel's roster verb %s is not named by the guard — the roster's business belongs to the application", verb)
		}
	}
}

// applicationOwnedVerbHits reports the application-owned verbs the source
// reaches through a selector.
//
// It sees a CALL (`k.Council(...)`) and a method VALUE (`k.Council` handed
// over as a function) alike: a verb that leaves the adapter only by becoming
// a value has not left the adapter.
//
// The boundary matters too — `CouncilDefaultMembers` is not `Council`, and a
// rule that cannot tell them apart flags a name it should not.
func applicationOwnedVerbHits(src string, verbs []string) []string {
	var hits []string
	for _, verb := range verbs {
		selector := regexp.MustCompile(`\.\s*` + regexp.QuoteMeta(verb) + `\b`)
		if selector.MatchString(src) {
			hits = append(hits, verb)
		}
	}
	return hits
}

func TestTheAdapterCallsNoApplicationOwnedVerbOnTheKernel(t *testing.T) {
	verbs := applicationOwnedVerbs(t)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, verb := range applicationOwnedVerbHits(string(raw), verbs) {
			t.Errorf("%s reaches the kernel's %s — that verb belongs to the application layer", name, verb)
		}
	}
}

// The guard is the thing that holds the edge, so the guard itself is held: a
// matcher that only saw calls would let a verb leave the adapter by becoming
// a value, and a matcher without a boundary would flag a name it should not.
func TestTheVerbGuardSeesAMethodValueAndKeepsItsBoundary(t *testing.T) {
	verbs := []string{"Council", "CouncilDefaultMembers", "RosterList"}
	if hits := applicationOwnedVerbHits("x := s.k.Council\n", verbs); len(hits) != 1 || hits[0] != "Council" {
		t.Errorf("hits = %v, want the method VALUE to be seen", hits)
	}
	if hits := applicationOwnedVerbHits("x := s.k.Council(ctx, corr, q, nil, 2)\n", verbs); len(hits) != 1 || hits[0] != "Council" {
		t.Errorf("hits = %v, want the call to be seen", hits)
	}
	if hits := applicationOwnedVerbHits("x := s.k.CouncilDefaultMembers()\n", verbs); len(hits) != 1 || hits[0] != "CouncilDefaultMembers" {
		t.Errorf("hits = %v, want CouncilDefaultMembers named as ITSELF — a name that starts with another verb's name is not that verb", hits)
	}
	if hits := applicationOwnedVerbHits("x := s.k.RosterList()\n", verbs); len(hits) != 1 || hits[0] != "RosterList" {
		t.Errorf("hits = %v, want RosterList to be seen", hits)
	}
	if hits := applicationOwnedVerbHits("x := s.rosterPorts().List\n", verbs); len(hits) != 0 {
		t.Errorf("hits = %v, want the application's own port to stay outside the rule", hits)
	}
}

// The control plane is an ADAPTER: it speaks the protocol and hands the work
// to the application layer.
//
// It may hold the kernel HANDLE — the runtime is what actually executes the
// runs — but every ROW it touches and every MECHANISM it uses must come from
// the contract layer or the application layer. A row the adapter can only
// name by naming the module is a row the adapter is not supposed to know,
// and a mechanism the adapter applies itself is a mechanism that can drift
// from the one the direct path uses.
//
// The GATE holds the adapter→module edge at PACKAGE granularity: as long as
// the adapter holds the kernel handle, the edge is allowed and the gate says
// nothing about WHICH symbols the adapter names beside it. This test holds
// the same edge at SYMBOL granularity — and it allows exactly one symbol:
// the handle itself.
//
// The file's own import says which name the runtime has in it, so the check
// is precise: a file that imports the kernel runtime may only use its
// `Kernel` type.
var kernelRuntimeImport = regexp.MustCompile(`(?:^|\n)\s*(?:(\w+)\s+)?"github\.com/agezt/agezt/kernel/runtime"`)

func TestTheAdapterNamesTheRuntimeOnlyAsAHandle(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)

		m := kernelRuntimeImport.FindStringSubmatch(src)
		if m == nil {
			continue // the file does not name the runtime at all
		}
		alias := m[1]
		if alias == "" {
			alias = "runtime"
		}
		selector := regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.([A-Za-z_][A-Za-z0-9_]*)`)
		for _, hit := range selector.FindAllStringSubmatch(src, -1) {
			if hit[1] == "Kernel" {
				continue
			}
			t.Errorf("%s names %s.%s — the adapter may hold the kernel handle, but every row and mechanism it touches must come from the contract or the application layer", name, alias, hit[1])
		}
	}
}
