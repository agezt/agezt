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

// kernelVerbsNamed are the kernel's methods whose name mentions the family.
//
// A family whose accessor file also holds other families' verbs cannot be
// found by file — the standing accessor file holds the data accessors too —
// so the families that live in a mixed file are found BY NAME.
//
// A family may have more than one name: the tools family is the tool verbs
// AND the MCP verbs, and the rule holds both.
func kernelVerbsNamed(t *testing.T, families ...string) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, family := range families {
			if strings.Contains(name, family) {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatalf("no %s verbs found — the kernel's %s verbs are gone?", strings.Join(families, "/"), strings.Join(families, "/"))
	}
	return verbs
}

// standingOwnedVerbs are the kernel's STANDING verbs: the library's store,
// its read snapshot and its four write verbs.
//
// The rule also sweeps in the two wake-provenance context readers
// (WakeContextStandingID, WakeContextStandingName). That is right: the adapter
// reads the wake provenance from the CONTRACT (agentapi.WakeContext), not
// through the kernel, and the standing order it names is the standing
// family's business.
func standingOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsNamed(t, "Standing")
}

// scheduleOwnedVerbs are the kernel's SCHEDULE verbs: the library's store,
// the engine that fires it and the wake-provenance context reader.
//
// The rule sweeps in the scheduler's own accessors (Scheduler,
// ScheduleEngine, SetScheduleEngine): the scheduler is the module's
// implementation, and the adapter must not name it either.
func scheduleOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsNamed(t, "Schedule")
}

// workflowOwnedVerbs are the kernel's WORKFLOW verbs: the library's store, the
// engine that runs it, and the eight verbs the workflow services need —
// save, restore, enable, remove, draft, refine, run and test.
//
// This is the first family whose application holds the HANDLE itself, so the
// rule matters most here: the application may reach the handle's workflow
// verbs through its port, and the adapter may not.
func workflowOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsNamed(t, "Workflow")
}

// toolsOwnedVerbs are the kernel's TOOL verbs: the tool registry, the
// script-tool forge, the tool runner and the MCP servers.
//
// The rule is BY NAME: a kernel verb that mentions a tool or an MCP server is
// the tools family's business, and a verb that lands in the kernel is guarded
// the day it lands.
//
// The rule also sweeps the two threshold verbs — MemoryDistillMinTools and
// SkillForgeMinTools. They are the memory and skill families' numbers rather
// than the tools family's, but the adapter must not name them either, and
// until those families are wired they have no rule of their own. When they
// are, the verb is named twice — a verb guarded twice is still guarded.
func toolsOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsNamed(t, "Tool", "MCP")
}

// okrAccessorFile is where the kernel's objective verbs live.
const okrAccessorFile = "../runtime/okr.go"

// okrAccessor finds the kernel's objective verbs in the file that declares
// them.
var okrAccessor = regexp.MustCompile(`func \(k \*Kernel\) ([A-Z][A-Za-z0-9_]*)\(`)

// okrOwnedVerbs are the kernel's OBJECTIVE verbs: the library's store, the
// rollup and the five writes.
//
// The rule reads the kernel's own objective file rather than the NAME: the
// rollup verb is called "Rollup" and says nothing about objectives, so a
// name rule would miss it — and a verb the rule misses is a verb the adapter
// may name.
func okrOwnedVerbs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(okrAccessorFile)
	if err != nil {
		t.Fatalf("read the kernel's objective accessors: %v", err)
	}
	matches := okrAccessor.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("no objective verbs found — the kernel's objective accessors moved?")
	}
	verbs := make([]string, 0, len(matches))
	for _, m := range matches {
		verbs = append(verbs, m[1])
	}
	return verbs
}

// workboardAccessor finds the kernel's workboard verbs in the file that
// declares them.
var workboardAccessor = regexp.MustCompile(`func \(k \*Kernel\) ([A-Z][A-Za-z0-9_]*)\(`)

// workboardAccessorFiles finds the kernel's workboard files.
//
// The family's verbs live in more than one file — the task lifecycle in one,
// the proof in another — so the rule reads them all. The pattern is a GLOB: a
// verb that lands in a new workboard file is guarded the day it lands.
func workboardAccessorFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "runtime", "workboard*.go"))
	if err != nil {
		t.Fatalf("glob the kernel's workboard accessors: %v", err)
	}
	var files []string
	for _, m := range matches {
		if strings.HasSuffix(m, "_test.go") {
			continue
		}
		files = append(files, m)
	}
	if len(files) == 0 {
		t.Fatal("no workboard accessor files found — the kernel's workboard verbs moved?")
	}
	return files
}

// workboardOwnedVerbs are the kernel's WORKBOARD verbs: the board's store, the
// task lifecycle, the relations and the proof.
//
// The rule reads the kernel's own workboard files rather than the NAME: the
// proof verb is called "ProveTask" and says nothing about the board, and a
// verb the rule misses is a verb the adapter may name.
func workboardOwnedVerbs(t *testing.T) []string {
	t.Helper()
	var verbs []string
	for _, file := range workboardAccessorFiles(t) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read the kernel's workboard accessors: %v", err)
		}
		for _, m := range workboardAccessor.FindAllStringSubmatch(string(raw), -1) {
			verbs = append(verbs, m[1])
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no workboard verbs found — the kernel's workboard accessors moved?")
	}
	return verbs
}

// configCenterOwnedVerbs are the kernel's CONFIG CENTER verbs: the center
// itself, which the adapter used to reach for whenever a family needed the
// config center's entries.
//
// The rule is BY NAME: a kernel verb that mentions the config center is the
// config center family's business, and a verb that lands in the kernel is
// guarded the day it lands.
func configCenterOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsNamed(t, "ConfigCenter")
}

// journalOwnedVerbs are the kernel's JOURNAL verbs: the event log every
// family reads.
//
// The rule is BY NAME: a kernel verb that mentions the journal is the journal
// family's business, and a verb that lands in the kernel is guarded the day
// it lands.
//
// This is the family with the most consumers: seventeen of them read the
// journal, and every one of them used to reach for the kernel to do it.
func journalOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsNamed(t, "Journal")
}

// busOwnedVerbs are the kernel's BUS verbs: the bus itself, the two verbs
// that publish through it, and every verb whose name says an event is being
// published.
//
// The rule is BY NAME: a kernel verb that mentions the bus, an event or a
// publish is the event bus family's business, and a verb that lands in the
// kernel is guarded the day it lands.
//
// The "Publish" name sweeps in the runtime's own publishes — the intent
// interpretation, the heuristic bypass, the context failure analysis. They
// publish through the bus, so the adapter must not name them either: the
// adapter publishes by ASKING the family, never by reaching for the kernel.
func busOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsNamed(t, "Bus", "Event", "Publish")
}

// applicationOwnedVerbs are all the verbs the application owns.
func applicationOwnedVerbs(t *testing.T) []string {
	t.Helper()
	verbs := append([]string{}, councilOwnedVerbs...)
	verbs = append(verbs, rosterOwnedVerbs(t)...)
	verbs = append(verbs, standingOwnedVerbs(t)...)
	verbs = append(verbs, scheduleOwnedVerbs(t)...)
	verbs = append(verbs, workflowOwnedVerbs(t)...)
	verbs = append(verbs, toolsOwnedVerbs(t)...)
	verbs = append(verbs, okrOwnedVerbs(t)...)
	verbs = append(verbs, configCenterOwnedVerbs(t)...)
	verbs = append(verbs, journalOwnedVerbs(t)...)
	verbs = append(verbs, busOwnedVerbs(t)...)
	return append(verbs, workboardOwnedVerbs(t)...)
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

// And the same for the STANDING family: every verb whose name mentions the
// standing library is the standing family's business, and the guard names it.
//
// The verbs are found HERE, by the kernel's own method set, not through the
// helper the guard uses — the expectation must not be derived by the code path
// it checks.
func TestTheGuardNamesEveryStandingVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		if !strings.Contains(name, "Standing") {
			continue
		}
		if !guarded[name] {
			t.Errorf("the kernel's standing verb %s is not named by the guard — the standing family's business belongs to the application", name)
		}
	}
}

// And the same for the SCHEDULE family: every verb whose name mentions the
// schedule library is the schedule family's business, and the guard names it.
//
// The verbs are found HERE, by the kernel's own method set, not through the
// helper the guard uses — the expectation must not be derived by the code path
// it checks.
func TestTheGuardNamesEveryScheduleVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		if !strings.Contains(name, "Schedule") {
			continue
		}
		if !guarded[name] {
			t.Errorf("the kernel's schedule verb %s is not named by the guard — the schedule family's business belongs to the application", name)
		}
	}
}

// And the same for the WORKFLOW family — the first one whose application
// holds the handle itself.
func TestTheGuardNamesEveryWorkflowVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		if !strings.Contains(name, "Workflow") {
			continue
		}
		if !guarded[name] {
			t.Errorf("the kernel's workflow verb %s is not named by the guard — the workflow family's business belongs to the application", name)
		}
	}
}

// And the same for the TOOLS family — the registry, the forge, the runner and
// the MCP servers.
func TestTheGuardNamesEveryToolsVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		if !strings.Contains(name, "Tool") && !strings.Contains(name, "MCP") {
			continue
		}
		if !guarded[name] {
			t.Errorf("the kernel's tools verb %s is not named by the guard — the tools family's business belongs to the application", name)
		}
	}
}

// And the same for the WORKBOARD family — read from the kernel's own
// workboard files, independently of the helper the guard uses.
func TestTheGuardNamesEveryWorkboardVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	// The files are found HERE, independently of the helper the guard uses: a
	// derivation that checks itself is not a derivation.
	matches, err := filepath.Glob("../runtime/workboard*.go")
	if err != nil {
		t.Fatalf("glob the kernel's workboard accessors: %v", err)
	}
	found := 0
	for _, file := range matches {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read the kernel's workboard accessors: %v", err)
		}
		declared := regexp.MustCompile(`func \(k \*Kernel\) ([A-Z][A-Za-z0-9_]*)\(`).FindAllStringSubmatch(string(raw), -1)
		if len(declared) == 0 {
			t.Fatalf("no workboard verbs found in %s — the kernel's workboard accessors moved?", file)
		}
		found += len(declared)
		for _, m := range declared {
			if !guarded[m[1]] {
				t.Errorf("the kernel's workboard verb %s is not named by the guard — the workboard family's business belongs to the application", m[1])
			}
		}
	}
	if found == 0 {
		t.Fatal("no workboard accessor files found — the kernel's workboard verbs moved?")
	}
}

// And the same for the JOURNAL family — the event log that seventeen
// families read.
func TestTheGuardNamesEveryJournalVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		if !strings.Contains(name, "Journal") {
			continue
		}
		if !guarded[name] {
			t.Errorf("the kernel's journal verb %s is not named by the guard — the journal family's business belongs to the application", name)
		}
	}
}

// And the same for the EVENT BUS family — the transport every family
// publishes through.
func TestTheGuardNamesEveryBusVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		if !strings.Contains(name, "Bus") && !strings.Contains(name, "Event") && !strings.Contains(name, "Publish") {
			continue
		}
		if !guarded[name] {
			t.Errorf("the kernel's bus verb %s is not named by the guard — the event bus family's business belongs to the application", name)
		}
	}
}

// And the same for the CONFIG CENTER family — the center the roster's
// teardown, the roster's cascade and the roster's permissions all read.
func TestTheGuardNamesEveryConfigCenterVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		if !strings.Contains(name, "ConfigCenter") {
			continue
		}
		if !guarded[name] {
			t.Errorf("the kernel's config center verb %s is not named by the guard — the config center family's business belongs to the application", name)
		}
	}
}

// And the same for the OBJECTIVE family — read from the kernel's own
// objective file, independently of the helper the guard uses.
func TestTheGuardNamesEveryObjectiveVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	raw, err := os.ReadFile(filepath.Join("..", "runtime", "okr.go"))
	if err != nil {
		t.Fatalf("read the kernel's objective accessors: %v", err)
	}
	declared := regexp.MustCompile(`func \(k \*Kernel\) ([A-Z][A-Za-z0-9_]*)\(`).FindAllStringSubmatch(string(raw), -1)
	if len(declared) == 0 {
		t.Fatal("no objective verbs found — the kernel's objective accessors moved?")
	}
	for _, m := range declared {
		if !guarded[m[1]] {
			t.Errorf("the kernel's objective verb %s is not named by the guard — the objective family's business belongs to the application", m[1])
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

// The HANDLE must not be handed to an application.
//
// The verb rule above cannot see a handle: `apptools.NewForgeLifecycle(k)`
// names no verb at all, so the guard would read it as clean — and the handle
// IS the family's whole capability surface, so handing it over is handing over
// the family. This rule closes that door: an application takes PORTS, and the
// handle the ports were filled from stops at the binding.
//
// The handle is the kernel itself, so the argument that hands it over is the
// one that ENDS with the handle's spelling. An argument that reaches THROUGH
// the handle for something else — the journal, say — is not the handle, and
// the rule leaves it alone.
//
// The rule covers every application that has declared its OWN ports: that is
// the port programme's own statement of who owns a kernel verb, and the list
// is read from the tree, so a family is covered the day it declares them.
var handleArgument = regexp.MustCompile(`(?:\bkernel\b|\.\s*k\b|\bk\b)$`)

// programmeApplications are the applications that declare their own ports.
func programmeApplications(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "app", "*", "ports.go"))
	if err != nil {
		t.Fatalf("glob the applications that declare their ports: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no application declares its own ports — the port programme moved?")
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, filepath.Base(filepath.Dir(m)))
	}
	return names
}

// applicationServiceArguments are the argument lists of an application's
// constructor calls in the source.
func applicationServiceArguments(src, alias string) []string {
	selector := regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.New[A-Za-z0-9_]*\(`)
	var lists []string
	for _, loc := range selector.FindAllStringIndex(src, -1) {
		if list, ok := balancedArguments(src, loc[1]-1); ok {
			lists = append(lists, list)
		}
	}
	return lists
}

// balancedArguments reads the argument list of the call whose opening
// parenthesis sits at the given index.
func balancedArguments(src string, open int) (string, bool) {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[open+1 : i], true
			}
		}
	}
	return "", false
}

// topLevelArguments splits an argument list at the commas that separate the
// arguments — a comma inside a call or a closure is not a separator.
func topLevelArguments(list string) []string {
	var args []string
	depth, start := 0, 0
	for i := 0; i < len(list); i++ {
		switch list[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, list[start:i])
				start = i + 1
			}
		}
	}
	return append(args, list[start:])
}

func TestTheAdapterHandsTheApplicationsPortsAndNotTheHandle(t *testing.T) {
	applications := programmeApplications(t)
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

		for _, application := range applications {
			importRE := regexp.MustCompile(`(?:^|\n)\s*(?:(\w+)\s+)?"github\.com/agezt/agezt/kernel/app/` + regexp.QuoteMeta(application) + `"`)
			m := importRE.FindStringSubmatch(src)
			if m == nil {
				continue // the file does not name this application at all
			}
			alias := m[1]
			if alias == "" {
				alias = application
			}
			for _, list := range applicationServiceArguments(src, alias) {
				for _, arg := range topLevelArguments(list) {
					arg = strings.TrimSpace(arg)
					if handleArgument.MatchString(arg) {
						t.Errorf("%s hands the kernel handle to the %s application (%s) — the family takes ports, not the handle it filled them from", name, application, arg)
					}
				}
			}
		}
	}
}

// The rule is held by the rule: a matcher that could not see the handle
// would let the family walk back in through the door the verb rule cannot
// see.
func TestTheHandleRuleSeesTheHandleAndLeavesTheReachedThingAlone(t *testing.T) {
	if !handleArgument.MatchString("host.kernel") {
		t.Error("the rule does not see the handle as a host field")
	}
	if !handleArgument.MatchString("s.k") {
		t.Error("the rule does not see the handle as a server field")
	}
	if !handleArgument.MatchString("ctx.Value(appHostKey{}).(appHost).kernel") {
		t.Error("the rule does not see the handle reached through the request")
	}
	if handleArgument.MatchString("ctx.Value(appHostKey{}).(appHost).kernel.Journal()") {
		t.Error("the rule took the journal for the handle — the journal is not the handle")
	}
	if handleArgument.MatchString("requestToolsPorts(ctx).Registry") {
		t.Error("the rule took a port for the handle — a port is what the family takes")
	}
	if got := topLevelArguments("a, f(b, c), d"); len(got) != 3 || strings.TrimSpace(got[1]) != "f(b, c)" {
		t.Errorf("topLevelArguments = %q, want the closure's comma left alone", got)
	}
	if got, ok := balancedArguments("f(a, g(b)) trailing", 1); !ok || got != "a, g(b)" {
		t.Errorf("balancedArguments = %q %v, want the call's own argument list", got, ok)
	}
}
