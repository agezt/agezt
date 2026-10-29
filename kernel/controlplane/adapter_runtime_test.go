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

// camelWords splits a camel-case name into its words.
//
// "SkillForgeMinTools" is three words — Skill, Forge, Min, Tools — and
// "MCPRegistration" is two — MCP, Registration. The split is what lets a
// family say "a verb whose name carries MY word" without claiming a verb
// that merely happens to contain the family's letters.
func camelWords(name string) []string {
	runes := []rune(name)
	var words []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		boundary := false
		switch {
		case cur >= 'A' && cur <= 'Z' && (prev < 'A' || prev > 'Z'):
			boundary = true
		case cur >= 'A' && cur <= 'Z' && prev >= 'A' && prev <= 'Z' && i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z':
			boundary = true
		}
		if boundary {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	return append(words, string(runes[start:]))
}

// kernelVerbsWord are the kernel's verbs whose name carries the family name as
// a COMPLETE word.
//
// The substring rule over-captures when a family name hides inside another
// word: "Forget" begins with "Forge", so the memory's MemoryForget would be
// claimed by the forge — and it is not the forge's business. A whole word is
// a word.
func kernelVerbsWord(t *testing.T, family string) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == family {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatalf("no %s verbs found — the kernel's %s verbs are gone?", family, family)
	}
	return verbs
}

// skillOwnedVerbs are the kernel's SKILL verbs: the forge — the daemon's skill
// library — and the verbs that say whether a run may forge.
//
// The rule is BY WORD: a kernel verb whose name carries "Forge" as a whole
// word is the forge's business, and a verb that lands in the kernel is guarded
// the day it lands.
//
// The sweep takes in MaybeForge, the runtime's own lifecycle hook: it decides
// whether a run's answer becomes a skill, and a skill is the forge's business.
//
// The TOOL forge is left out: the script-tool forge is the tools family's own
// verb, and a verb that another family names as a whole word is that family's
// business. The skill family's own thresholds — SkillForgeMinTools — stay
// here: a threshold on forging is the forge's business, even though the
// tools' rule happens to catch it too.
func skillOwnedVerbs(t *testing.T) []string {
	t.Helper()
	var verbs []string
	for _, verb := range kernelVerbsWord(t, "Forge") {
		other := false
		for _, word := range camelWords(verb) {
			if word == "Tool" || word == "MCP" {
				other = true
			}
		}
		if other {
			continue
		}
		verbs = append(verbs, verb)
	}
	if len(verbs) == 0 {
		t.Fatal("no skill verbs found — the kernel's skill verbs are gone?")
	}
	return verbs
}

// memoryOwnedVerbs are the kernel's MEMORY verbs: the manager, the active
// records, the forget, the profile and the searches.
//
// The rule is BY WORD: a kernel verb whose name carries "Memory" as a whole
// word is the memory family's business, and a verb that lands in the kernel is
// guarded the day it lands.
//
// The sweep takes in the distillation verbs — the brain, the facts and the
// profile: they consolidate the memory's own records, so they are the memory's
// business even though the runtime calls them. The family answers to TWO
// names, "Memory" and "Distill", because the checked distillation surface
// (DistillBrain, DistillProfile) does not say "memory" in its name.
func memoryOwnedVerbs(t *testing.T) []string {
	t.Helper()
	verbs := kernelVerbsWord(t, "Memory")
	for _, verb := range kernelVerbsWord(t, "Distill") {
		seen := false
		for _, owned := range verbs {
			if owned == verb {
				seen = true
			}
		}
		if !seen {
			verbs = append(verbs, verb)
		}
	}
	return verbs
}

// edictOwnedVerbs are the kernel's POLICY verbs: the policy engine every
// capability is decided against.
//
// The rule is BY WORD: a kernel verb whose name carries "Edict" as a whole word
// is the policy family's business, and a verb that lands in the kernel is
// guarded the day it lands.
//
// The whole-word rule is the sharp one here: "Edict" hides inside no other
// word of the kernel's, and a substring rule would claim a verb whose name
// merely begins with the same letters.
func edictOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsWord(t, "Edict")
}

// providerOwnedVerbs are the kernel's PROVIDER verbs: the live provider every
// routing and spending question goes through.
//
// The rule is BY WORD: a kernel verb whose name carries "Provider" as a whole
// word is the provider family's business, and a verb that lands in the kernel
// is guarded the day it lands.
//
// The daemon's live provider is the governor, and the governor carries three
// faces the adapter used to reach one at a time: the routing tables it routes
// by, the routing view it reports and the spend it keeps. One door, four
// consumers — and the door belongs to the family.
func providerOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsWord(t, "Provider")
}

// baseDirOwnedVerbs are the kernel's DATA ROOT verbs: where the daemon keeps
// everything.
//
// The rule is by WORD, and the family name is TWO words: camel-case splits
// "BaseDir" into "Base" and "Dir", so a rule that looked for the whole name
// would never match. The rule therefore looks for a verb that carries BOTH
// words.
//
// Neither half is enough on its own. "Base" alone would be too loose, and the
// compound "BaseDir" as a SUBSTRING would over-capture: "RebaseDir" begins
// with "Base" and is not the data root's business. "RebaseDir" splits into
// "Rebase" and "Dir", and carries no "Base" word at all.
//
// The family is its own family on purpose: the data root is not the journal's
// business, nor the sandbox's, nor the storage's — it is the place all of
// them keep their files, and it is the place a tenant's own kernel keeps the
// tenant's.
func baseDirOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		hasBase, hasDir := false, false
		for _, word := range camelWords(name) {
			if word == "Base" {
				hasBase = true
			}
			if word == "Dir" {
				hasDir = true
			}
		}
		if hasBase && hasDir {
			verbs = append(verbs, name)
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no data-root verbs found — the kernel's data-root verbs are gone?")
	}
	return verbs
}

// correlationOwnedVerbs are the kernel's CORRELATION verbs: the identity one
// operation carries.
//
// The rule is BY WORD: a kernel verb whose name carries "Correlation" as a
// whole word is the correlation family's business, and a verb that lands in
// the kernel is guarded the day it lands.
//
// The sweep takes in three verbs: the mint, the actor decoration and the wake
// provenance's parent correlation. The third says nothing about minting — it
// is a reader — but it reads the same identity, so it is the same family's
// business.
func correlationOwnedVerbs(t *testing.T) []string {
	t.Helper()
	return kernelVerbsWord(t, "Correlation")
}

// catalogOwnedVerbs are the kernel's CATALOG verbs: the catalog directory the
// daemon keeps its provider and model catalog in, and the refresh that
// re-reads it.
//
// The rule is BY WORD, and the family answers to TWO names, "Catalog" and
// "Reload": the checked refresh surface (Reload, ReloadCatalog) does not say
// "catalog" in its name, and the refresh IS the catalog's own — it re-reads the
// catalog from disk and rebuilds the live provider registry from what it read.
//
// The two names are UNIONED, not chosen between: "ReloadCatalog" carries both,
// and a family that took only one of them would leave the other unguarded.
func catalogOwnedVerbs(t *testing.T) []string {
	t.Helper()
	verbs := kernelVerbsWord(t, "Catalog")
	for _, verb := range kernelVerbsWord(t, "Reload") {
		seen := false
		for _, owned := range verbs {
			if owned == verb {
				seen = true
			}
		}
		if !seen {
			verbs = append(verbs, verb)
		}
	}
	return verbs
}

// artifactOwnedVerbs are the kernel's ARTIFACT verbs: the content-addressed
// store the loop offloads tool outputs into, and the metadata index over it.
//
// The rule is by WORD, and the family answers to TWO names, "Artifact" and
// "Artifacts" — because the camel-case splitter reads "ArtifactIndex" as
// "Artifact" + "Index", while "Artifacts" is ONE word to it: a plural 's' does
// not start a new word.
//
// A family that looked for a single name would guard either the store and the
// index, or the plural, and never both — and the one it left out would be a
// verb the adapter may name.
func artifactOwnedVerbs(t *testing.T) []string {
	t.Helper()
	verbs := kernelVerbsWord(t, "Artifact")
	for _, verb := range kernelVerbsWord(t, "Artifacts") {
		seen := false
		for _, owned := range verbs {
			if owned == verb {
				seen = true
			}
		}
		if !seen {
			verbs = append(verbs, verb)
		}
	}
	return verbs
}

// pluginOwnedVerbs are the kernel's PLUGIN verbs: the external-plugin manifest
// the daemon was given at Open.
//
// The rule is by WORD, and the family answers to **two** names, "Plugin" and
// "Plugins" — for the same reason the artifacts family answers to "Artifact"
// and "Artifacts": the camel-case splitter reads "PluginCount" as "Plugin" +
// "Count", while "Plugins" is ONE word to it.
//
// The rule is written out rather than taken from the helper, because the
// helper FAILS when a name finds no verb at all — and "Plugin" finds none
// today. A rule that cannot be asked about a name it has not seen yet is a
// rule that breaks on the day the name arrives.
func pluginOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == "Plugin" || word == "Plugins" {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no plugin verbs found — the kernel's plugin verbs are gone?")
	}
	return verbs
}

// marketOwnedVerbs are the kernel's MARKETPLACE verbs: the manager over the
// daemon's own library of packs — the built-in Official library and the sources
// the operator adds.
//
// The rule is by WORD: a kernel verb whose name carries "Market" as a whole
// word is the marketplace family's business. That catches BOTH the accessor
// ("Market") and the setter ("SetMarket" — the camel-case splitter reads it as
// "Set" + "Market"), and it catches a marketplace verb the day it lands.
//
// The rule is written out rather than taken from the helper, so that the
// family's own shape — one name that covers two verbs — is visible here and
// can be held: a helper that only ever answered with the exact name would
// leave the setter unguarded.
func marketOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == "Market" {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no marketplace verbs found — the kernel's marketplace verbs are gone?")
	}
	return verbs
}

// approvalOwnedVerbs are the kernel's APPROVAL verbs: the human-in-the-loop
// queue the daemon owns — the requests waiting for an operator and the
// decision that answers them.
//
// The rule is by WORD: a kernel verb whose name carries "Approvals" as a
// whole word is the approvals family's business. The queue is the approvals
// family's own, and a family that asked the kernel for it would be reaching
// across the boundary.
func approvalOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == "Approvals" {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no approval verbs found — the kernel's approval verbs are gone?")
	}
	return verbs
}

// worldOwnedVerbs are the kernel's WORLD verbs: the graph of entities and the
// relations between them — what the operator's world is made of.
//
// The rule is by WORD: a kernel verb whose name carries "World" as a whole
// word is the world family's business.
func worldOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == "World" {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no world verbs found — the kernel's world verbs are gone?")
	}
	return verbs
}

// tasteOwnedVerbs are the kernel's TASTE verbs: the curated exemplar library
// the daemon keeps — the examples a run may imitate.
//
// The rule is by WORD: a kernel verb whose name carries "Taste" as a whole
// word is the taste family's business.
func tasteOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == "Taste" {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no taste verbs found — the kernel's taste verbs are gone?")
	}
	return verbs
}

// personaOwnedVerbs are the kernel's IDENTITY verbs: the daemon's default
// identity — the model the runs fall back to and the system instructions they
// fall back to.
//
// The rule is by WORD, over TWO words: "System" and "Model". The persona
// family is the owner's chat defaults, and the daemon's default identity is
// its business.
//
// FOUR verbs carry those words and are NOT the identity's: "ResolveRunModel"
// and "currentTaskModelChain" are the ROUTING's business — which model a run
// resolves — "SystemTaskPorts" is the SYSTEM TASK family's, and
// "VisionModel" is the daemon's vision SIDECAR — the model that captions
// images for a run whose active model cannot see them. A rule that swept them
// in would make the persona family claim four other families' verbs, and a
// family that claims everything claims nothing.
//
// The CONTEXT helpers are not the identity's either: the system agent from the
// context is the roster's business, and the model chain from the context is
// the routing's.
func personaOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "System" || word == "Model" {
				carries = true
			}
		}
		if !carries || strings.HasSuffix(name, "FromCtx") {
			continue
		}
		switch name {
		case "ResolveRunModel", "currentTaskModelChain", "SystemTaskPorts", "VisionModel":
			continue
		}
		verbs = append(verbs, name)
	}
	if len(verbs) == 0 {
		t.Fatal("no identity verbs found — the kernel's identity verbs are gone?")
	}
	return verbs
}

// stateOwnedVerbs are the kernel's STATE verbs: the daemon's own key/value
// file — the namespaces and the keys under them.
//
// The rule is by WORD: a kernel verb whose name carries "State" as a whole
// word is the state family's business.
//
// THREE verbs carry it and are NOT the state family's: "CleanupRunState",
// "SetupRunState" and "RunControlState" are the RUN family's business — the
// state a run leaves behind and the control that stops it. A rule that swept
// them in would make the state family claim three other families' verbs.
func stateOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "State" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		switch name {
		case "CleanupRunState", "SetupRunState", "RunControlState":
			continue
		}
		verbs = append(verbs, name)
	}
	if len(verbs) == 0 {
		t.Fatal("no state verbs found — the kernel's state verbs are gone?")
	}
	return verbs
}

// seatOwnedVerbs are the kernel's SEAT verbs: the execution-seat catalog the
// daemon owns — the seats it offers, seeded and custom.
//
// The rule is by WORD: a kernel verb whose name carries "Seats" as a whole
// word is the seats family's business.
//
// The word is the PLURAL "Seats", because that is the word the kernel's own
// verb carries: the camel-case splitter reads "Seats" as ONE word — a trailing
// plural "s" does not start a new word — so a rule written as "Seat" would
// match nothing at all and the guard would fail on the day it is written.
func seatOwnedVerbs(t *testing.T) []string {
	t.Helper()
	kernel := reflect.TypeOf(&runtime.Kernel{})
	var verbs []string
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == "Seats" {
				verbs = append(verbs, name)
				break
			}
		}
	}
	if len(verbs) == 0 {
		t.Fatal("no seat verbs found — the kernel's seat verbs are gone?")
	}
	return verbs
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
	verbs = append(verbs, skillOwnedVerbs(t)...)
	verbs = append(verbs, memoryOwnedVerbs(t)...)
	verbs = append(verbs, edictOwnedVerbs(t)...)
	verbs = append(verbs, providerOwnedVerbs(t)...)
	verbs = append(verbs, baseDirOwnedVerbs(t)...)
	verbs = append(verbs, correlationOwnedVerbs(t)...)
	verbs = append(verbs, catalogOwnedVerbs(t)...)
	verbs = append(verbs, artifactOwnedVerbs(t)...)
	verbs = append(verbs, pluginOwnedVerbs(t)...)
	verbs = append(verbs, marketOwnedVerbs(t)...)
	verbs = append(verbs, approvalOwnedVerbs(t)...)
	verbs = append(verbs, worldOwnedVerbs(t)...)
	verbs = append(verbs, tasteOwnedVerbs(t)...)
	verbs = append(verbs, personaOwnedVerbs(t)...)
	verbs = append(verbs, stateOwnedVerbs(t)...)
	verbs = append(verbs, seatOwnedVerbs(t)...)
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

// The rule must be SHARP, not only broad: a family name that hides inside
// another word is not that word.
//
// "Forget" begins with "Forge", so a substring rule would hand the memory's
// MemoryForget to the forge — and forgetting a memory is not the forge's
// business. The rule is by WHOLE WORD, and the rule is held by the rule: the
// memory's verbs are found HERE, by the kernel's own method set, and the
// skill family's list must not carry any of them.
func TestTheSkillFamilyDoesNotClaimTheMemorysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	memoryVerbs := map[string]bool{}
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		for _, word := range camelWords(name) {
			if word == "Memory" {
				memoryVerbs[name] = true
			}
		}
	}
	if len(memoryVerbs) == 0 {
		t.Fatal("no memory verbs found — the kernel's memory verbs are gone?")
	}
	for _, verb := range skillOwnedVerbs(t) {
		if memoryVerbs[verb] {
			t.Errorf("the skill family claims the kernel's %s — a memory is not a skill", verb)
		}
	}
}

// And the same for the MEMORY family — the daemon's records.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Memory" or "Distill" as a whole
// word is the memory's business. The expectation is not derived through the
// helper the guard uses.
func TestTheGuardNamesEveryMemoryVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Memory" || word == "Distill" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's memory verb %s is not named by the guard — the memory family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no memory verbs found — the kernel's memory verbs are gone?")
	}
}

// And the same for the SKILL family — the daemon's skill library.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Forge" as a whole word is the
// forge's business, and a verb the tools family already names is the tools
// family's. The expectation is not derived through the helper the guard uses.
func TestTheGuardNamesEverySkillVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Forge" {
				carries = true
			}
			if word == "Tool" || word == "MCP" {
				carries = false
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's skill verb %s is not named by the guard — the skill family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no skill verbs found — the kernel's skill verbs are gone?")
	}
}

// And the same for the POLICY family — the engine every capability is decided
// against.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Edict" as a whole word is the
// policy's business. The expectation is not derived through the helper the
// guard uses.
func TestTheGuardNamesEveryEdictVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Edict" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's policy verb %s is not named by the guard — the policy family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no policy verbs found — the kernel's policy verbs are gone?")
	}
}

// And the POLICY family must not claim another family's verbs: a policy
// decision is not a tool's business, a memory's business, or the bus's.
//
// The rule must be BOTH broad and sharp. Broad: every policy verb is guarded
// (the test above). Sharp: the policy's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
//
// The other families' verbs are found by THEIR OWN rules, read here, so the
// check does not stand on the helper the guard uses.
func TestTheEdictFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	edict := edictOwnedVerbs(t)
	if len(edict) == 0 {
		t.Fatal("the policy family names no verb — the policy family's business belongs to the application")
	}
	for _, verb := range edict {
		if others[verb] {
			t.Errorf("the policy family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	kernelVerbs := kernel.NumMethod()
	if len(edict) == kernelVerbs {
		t.Errorf("the policy family claims all %d kernel verbs — a family that claims everything claims nothing", kernelVerbs)
	}
}

// And the same for the PROVIDER family — the live provider the routing
// tables, the routing view and the spend all come through.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Provider" as a whole word is the
// provider's business. The expectation is not derived through the helper the
// guard uses.
func TestTheGuardNamesEveryProviderVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Provider" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's provider verb %s is not named by the guard — the provider family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no provider verbs found — the kernel's provider verbs are gone?")
	}
}

// And the PROVIDER family must not claim another family's verbs: the live
// provider is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every provider verb is
// guarded (the test above). Sharp: the provider's list carries nothing that
// belongs to someone else, and it is a strict subset of the kernel's verbs —
// a rule that only ever grows is a rule that eventually guards the whole
// kernel, and a guard that guards the whole kernel guards nothing.
func TestTheProviderFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	provider := providerOwnedVerbs(t)
	if len(provider) == 0 {
		t.Fatal("the provider family names no verb — the provider family's business belongs to the application")
	}
	for _, verb := range provider {
		if others[verb] {
			t.Errorf("the provider family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(provider) == kernel.NumMethod() {
		t.Errorf("the provider family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the DATA ROOT family — where the daemon keeps everything.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries BOTH the "Base" and the "Dir"
// words — the compound is two words, and neither half is the family. The
// expectation is not derived through the helper the guard uses.
func TestTheGuardNamesEveryBaseDirVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		hasBase, hasDir := false, false
		for _, word := range camelWords(name) {
			if word == "Base" {
				hasBase = true
			}
			if word == "Dir" {
				hasDir = true
			}
		}
		if !hasBase || !hasDir {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's data-root verb %s is not named by the guard — the data root's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no data-root verbs found — the kernel's data-root verbs are gone?")
	}
}

// And the DATA ROOT family must not claim another family's verbs.
//
// The rule must be BOTH broad and sharp: broad (every data-root verb is
// guarded, the test above) and sharp (the data root's list carries nothing
// that belongs to someone else, and it is a strict subset of the kernel's
// verbs).
func TestTheBaseDirFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	baseDir := baseDirOwnedVerbs(t)
	if len(baseDir) == 0 {
		t.Fatal("the data-root family names no verb — the data root's business belongs to the application")
	}
	for _, verb := range baseDir {
		if others[verb] {
			t.Errorf("the data-root family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(baseDir) == kernel.NumMethod() {
		t.Errorf("the data-root family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the CORRELATION family — the identity one operation
// carries.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Correlation" as a whole word is
// the correlation family's business. The expectation is not derived through
// the helper the guard uses.
func TestTheGuardNamesEveryCorrelationVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Correlation" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's correlation verb %s is not named by the guard — the correlation family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no correlation verbs found — the kernel's correlation verbs are gone?")
	}
}

// And the CORRELATION family must not claim another family's verbs: a
// correlation is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every correlation verb is
// guarded (the test above). Sharp: the correlation's list carries nothing that
// belongs to someone else, and it is a strict subset of the kernel's verbs — a
// rule that only ever grows is a rule that eventually guards the whole kernel,
// and a guard that guards the whole kernel guards nothing.
func TestTheCorrelationFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	correlation := correlationOwnedVerbs(t)
	if len(correlation) == 0 {
		t.Fatal("the correlation family names no verb — the correlation family's business belongs to the application")
	}
	for _, verb := range correlation {
		if others[verb] {
			t.Errorf("the correlation family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(correlation) == kernel.NumMethod() {
		t.Errorf("the correlation family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the CATALOG family — the catalog directory and the refresh
// that re-reads it.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Catalog" or "Reload" as a whole
// word is the catalog family's business. The expectation is not derived through
// the helper the guard uses.
func TestTheGuardNamesEveryCatalogVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Catalog" || word == "Reload" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's catalog verb %s is not named by the guard — the catalog family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no catalog verbs found — the kernel's catalog verbs are gone?")
	}
}

// And the CATALOG family must not claim another family's verbs: the catalog
// directory is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every catalog verb is guarded
// (the test above). Sharp: the catalog's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestTheCatalogFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	catalog := catalogOwnedVerbs(t)
	if len(catalog) == 0 {
		t.Fatal("the catalog family names no verb — the catalog family's business belongs to the application")
	}
	for _, verb := range catalog {
		if others[verb] {
			t.Errorf("the catalog family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(catalog) == kernel.NumMethod() {
		t.Errorf("the catalog family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the ARTIFACT family — the content-addressed store and the
// metadata index over it.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Artifact" or "Artifacts" as a whole
// word is the artifacts family's business. The expectation is not derived
// through the helper the guard uses.
func TestTheGuardNamesEveryArtifactVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Artifact" || word == "Artifacts" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's artifact verb %s is not named by the guard — the artifacts family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no artifact verbs found — the kernel's artifact verbs are gone?")
	}
}

// And the ARTIFACT family must not claim another family's verbs: the artifact
// store is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every artifact verb is guarded
// (the test above). Sharp: the artifact's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestTheArtifactFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	artifact := artifactOwnedVerbs(t)
	if len(artifact) == 0 {
		t.Fatal("the artifact family names no verb — the artifacts family's business belongs to the application")
	}
	for _, verb := range artifact {
		if others[verb] {
			t.Errorf("the artifact family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(artifact) == kernel.NumMethod() {
		t.Errorf("the artifact family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the PLUGIN family — the external-plugin manifest the daemon
// was given at Open.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Plugin" or "Plugins" as a whole
// word is the plugins family's business. The expectation is not derived through
// the helper the guard uses.
func TestTheGuardNamesEveryPluginVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Plugin" || word == "Plugins" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's plugin verb %s is not named by the guard — the plugins family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no plugin verbs found — the kernel's plugin verbs are gone?")
	}
}

// And the PLUGIN family must not claim another family's verbs: the plugin
// manifest is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every plugin verb is guarded
// (the test above). Sharp: the plugin's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestThePluginFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	plugin := pluginOwnedVerbs(t)
	if len(plugin) == 0 {
		t.Fatal("the plugin family names no verb — the plugins family's business belongs to the application")
	}
	for _, verb := range plugin {
		if others[verb] {
			t.Errorf("the plugin family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(plugin) == kernel.NumMethod() {
		t.Errorf("the plugin family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the MARKETPLACE family — the manager over the daemon's own
// library of packs.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Market" as a whole word is the
// marketplace family's business. The expectation is not derived through the
// helper the guard uses.
func TestTheGuardNamesEveryMarketVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Market" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's marketplace verb %s is not named by the guard — the marketplace family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no marketplace verbs found — the kernel's marketplace verbs are gone?")
	}
}

// And the MARKETPLACE family must not claim another family's verbs: the
// marketplace's library is not a tool's business, a memory's business, or the
// policy's.
//
// The rule must be BOTH broad and sharp. Broad: every marketplace verb is
// guarded (the test above). Sharp: the marketplace's list carries nothing that
// belongs to someone else, and it is a strict subset of the kernel's verbs — a
// rule that only ever grows is a rule that eventually guards the whole kernel,
// and a guard that guards the whole kernel guards nothing.
func TestTheMarketFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		pluginOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	market := marketOwnedVerbs(t)
	if len(market) == 0 {
		t.Fatal("the marketplace family names no verb — the marketplace family's business belongs to the application")
	}
	for _, verb := range market {
		if others[verb] {
			t.Errorf("the marketplace family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(market) == kernel.NumMethod() {
		t.Errorf("the marketplace family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the APPROVALS family — the human-in-the-loop queue the
// daemon owns.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Approvals" as a whole word is the
// approvals family's business. The expectation is not derived through the
// helper the guard uses.
func TestTheGuardNamesEveryApprovalVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Approvals" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's approval verb %s is not named by the guard — the approvals family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no approval verbs found — the kernel's approval verbs are gone?")
	}
}

// And the APPROVALS family must not claim another family's verbs: the queue is
// not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every approval verb is guarded
// (the test above). Sharp: the approvals' list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestTheApprovalFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		pluginOwnedVerbs(t),
		marketOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	approval := approvalOwnedVerbs(t)
	if len(approval) == 0 {
		t.Fatal("the approvals family names no verb — the approvals family's business belongs to the application")
	}
	for _, verb := range approval {
		if others[verb] {
			t.Errorf("the approvals family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(approval) == kernel.NumMethod() {
		t.Errorf("the approvals family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the WORLD family — the graph of entities and the relations
// between them.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "World" as a whole word is the world
// family's business. The expectation is not derived through the helper the
// guard uses.
func TestTheGuardNamesEveryWorldVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "World" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's world verb %s is not named by the guard — the world family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no world verbs found — the kernel's world verbs are gone?")
	}
}

// And the WORLD family must not claim another family's verbs: the world model
// is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every world verb is guarded
// (the test above). Sharp: the world's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestTheWorldFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		pluginOwnedVerbs(t),
		marketOwnedVerbs(t),
		approvalOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	world := worldOwnedVerbs(t)
	if len(world) == 0 {
		t.Fatal("the world family names no verb — the world family's business belongs to the application")
	}
	for _, verb := range world {
		if others[verb] {
			t.Errorf("the world family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(world) == kernel.NumMethod() {
		t.Errorf("the world family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the TASTE family — the curated exemplar library the daemon
// keeps.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Taste" as a whole word is the taste
// family's business. The expectation is not derived through the helper the
// guard uses.
func TestTheGuardNamesEveryTasteVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Taste" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's taste verb %s is not named by the guard — the taste family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no taste verbs found — the kernel's taste verbs are gone?")
	}
}

// And the TASTE family must not claim another family's verbs: the exemplar
// library is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every taste verb is guarded
// (the test above). Sharp: the taste's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestTheTasteFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		pluginOwnedVerbs(t),
		marketOwnedVerbs(t),
		approvalOwnedVerbs(t),
		worldOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	taste := tasteOwnedVerbs(t)
	if len(taste) == 0 {
		t.Fatal("the taste family names no verb — the taste family's business belongs to the application")
	}
	for _, verb := range taste {
		if others[verb] {
			t.Errorf("the taste family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(taste) == kernel.NumMethod() {
		t.Errorf("the taste family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the PERSONA family — the owner's chat defaults, and the
// daemon's default identity.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "System" or "Model" as a whole word
// is the identity's business, and the CONTEXT helpers are not.
func TestTheGuardNamesEveryPersonaVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "System" || word == "Model" {
				carries = true
			}
		}
		if !carries || strings.HasSuffix(name, "FromCtx") {
			continue
		}
		switch name {
		case "ResolveRunModel", "currentTaskModelChain", "SystemTaskPorts", "VisionModel":
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's identity verb %s is not named by the guard — the persona family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no identity verbs found — the kernel's identity verbs are gone?")
	}
}

// And the PERSONA family must not claim another family's verbs: the daemon's
// default identity is not a tool's business, a memory's business, or the
// policy's.
//
// The rule must be BOTH broad and sharp. Broad: every identity verb is guarded
// (the test above). Sharp: the persona's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestThePersonaFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		pluginOwnedVerbs(t),
		marketOwnedVerbs(t),
		approvalOwnedVerbs(t),
		worldOwnedVerbs(t),
		tasteOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	persona := personaOwnedVerbs(t)
	if len(persona) == 0 {
		t.Fatal("the persona family names no verb — the persona family's business belongs to the application")
	}
	for _, verb := range persona {
		if others[verb] {
			t.Errorf("the persona family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(persona) == kernel.NumMethod() {
		t.Errorf("the persona family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the STATE family — the daemon's own key/value file.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "State" as a whole word is the
// state's business, and the RUN's state verbs are not.
func TestTheGuardNamesEveryStateVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "State" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		switch name {
		case "CleanupRunState", "SetupRunState", "RunControlState":
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's state verb %s is not named by the guard — the state family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no state verbs found — the kernel's state verbs are gone?")
	}
}

// And the STATE family must not claim another family's verbs: the daemon's
// key/value file is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every state verb is guarded
// (the test above). Sharp: the state's list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestTheStateFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		pluginOwnedVerbs(t),
		marketOwnedVerbs(t),
		approvalOwnedVerbs(t),
		worldOwnedVerbs(t),
		tasteOwnedVerbs(t),
		personaOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	state := stateOwnedVerbs(t)
	if len(state) == 0 {
		t.Fatal("the state family names no verb — the state family's business belongs to the application")
	}
	for _, verb := range state {
		if others[verb] {
			t.Errorf("the state family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	// And the RUN's state verbs are the run's own: the state a run leaves
	// behind and the control that stops it are the run family's business, and
	// the state family's rule says so out loud.
	for _, verb := range []string{"CleanupRunState", "SetupRunState", "RunControlState"} {
		for _, claimed := range state {
			if claimed == verb {
				t.Errorf("the state family claims the kernel's %s — that verb is the RUN family's business", verb)
			}
		}
	}
	if len(state) == kernel.NumMethod() {
		t.Errorf("the state family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
	}
}

// And the same for the SEATS family — the execution-seat catalog the daemon
// owns.
//
// The verbs are found HERE, by the kernel's own method set and by the rule the
// family states: a verb whose name carries "Seats" as a whole word is the
// seats family's business. The expectation is not derived through the helper
// the guard uses.
func TestTheGuardNamesEverySeatVerbTheKernelHas(t *testing.T) {
	guarded := map[string]bool{}
	for _, verb := range applicationOwnedVerbs(t) {
		guarded[verb] = true
	}
	kernel := reflect.TypeOf(&runtime.Kernel{})
	found := 0
	for i := 0; i < kernel.NumMethod(); i++ {
		name := kernel.Method(i).Name
		carries := false
		for _, word := range camelWords(name) {
			if word == "Seats" {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found++
		if !guarded[name] {
			t.Errorf("the kernel's seat verb %s is not named by the guard — the seats family's business belongs to the application", name)
		}
	}
	if found == 0 {
		t.Fatal("no seat verbs found — the kernel's seat verbs are gone?")
	}
}

// And the SEATS family must not claim another family's verbs: the execution
// seat catalog is not a tool's business, a memory's business, or the policy's.
//
// The rule must be BOTH broad and sharp. Broad: every seat verb is guarded
// (the test above). Sharp: the seats' list carries nothing that belongs to
// someone else, and it is a strict subset of the kernel's verbs — a rule that
// only ever grows is a rule that eventually guards the whole kernel, and a
// guard that guards the whole kernel guards nothing.
func TestTheSeatFamilyClaimsNoOtherFamilysVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	others := map[string]bool{}
	for _, list := range [][]string{
		councilOwnedVerbs,
		rosterOwnedVerbs(t),
		standingOwnedVerbs(t),
		scheduleOwnedVerbs(t),
		workflowOwnedVerbs(t),
		toolsOwnedVerbs(t),
		okrOwnedVerbs(t),
		configCenterOwnedVerbs(t),
		journalOwnedVerbs(t),
		busOwnedVerbs(t),
		skillOwnedVerbs(t),
		memoryOwnedVerbs(t),
		edictOwnedVerbs(t),
		providerOwnedVerbs(t),
		baseDirOwnedVerbs(t),
		correlationOwnedVerbs(t),
		catalogOwnedVerbs(t),
		artifactOwnedVerbs(t),
		pluginOwnedVerbs(t),
		marketOwnedVerbs(t),
		approvalOwnedVerbs(t),
		worldOwnedVerbs(t),
		tasteOwnedVerbs(t),
		personaOwnedVerbs(t),
		stateOwnedVerbs(t),
		workboardOwnedVerbs(t),
	} {
		for _, verb := range list {
			others[verb] = true
		}
	}
	seats := seatOwnedVerbs(t)
	if len(seats) == 0 {
		t.Fatal("the seats family names no verb — the seats family's business belongs to the application")
	}
	for _, verb := range seats {
		if others[verb] {
			t.Errorf("the seats family claims the kernel's %s — that verb is another family's business", verb)
		}
	}
	if len(seats) == kernel.NumMethod() {
		t.Errorf("the seats family claims all %d kernel verbs — a family that claims everything claims nothing", kernel.NumMethod())
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
