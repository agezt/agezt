// SPDX-License-Identifier: MIT

package runtime

// The daemon's DEAD SURFACE — the ratchet of W3.76k.
//
// The daemon is the host: it composes the modules, holds the ports and
// answers the verbs the layers above call. A verb the daemon keeps that
// NOBODY calls is not a door — it is a second, unreachable copy of a surface,
// and it is what the W3 exit has to shave off: the target architecture
// dissolves the kernel (284 methods) into the modules that own the families.
//
// W3.76k measured the daemon's 238 public verbs and found the ones no
// caller reached. Sixteen of them were unreachable:
//
//   - three had NO caller at all (the artifact store getter, the bus
//     publisher the deleted Accessor sub-package owned, the scheduler
//     getter);
//   - four were the daemon's forwarding copies of the RUNS MODULE's
//     vocabulary (the run timeout, the auto-approve merge) — and they were
//     also four VERBS of the runner's KernelAPI port that the runner never
//     opened;
//   - six were the daemon's memory reads and writes that nobody asked for
//     (the memory family is the knowledge module's);
//   - one was the OKR "fetch the objective by id and roll it up" verb;
//   - two were the wiring setters the composition root replaced with the
//     Config builders (the forge and the MCP registry services);
//   - two were the daemon's own MCP readers, whose row surface above them
//     (MCPServers / MCPServer) answers the same rows.
//
// The ratchet pins that: the names do not come back, and the daemon's public
// surface does not grow past the number this slice left behind.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting one of the names back has to
// make this file fail.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// retiredVerbs are the daemon's public verbs W3.76k retired. Each one had no
// caller outside the daemon; each one is gone from the daemon's public
// method set.
var retiredVerbs = []string{
	// No caller at all.
	"ArtifactStore",
	"PublishBusEvent",
	"Scheduler",
	// The daemon's forwarding copies of the runs module's vocabulary.
	"RunTimeoutFromCtx",
	"MergeAutoApproveCapabilities",
	// The daemon's memory reads and writes nobody asked for.
	"MemoryActive",
	"MemoryProfileText",
	"MemorySearchScoped",
	"MemoryRecallScoped",
	"MemoryForget",
	"MemoryDistillFacts",
	// The daemon's second spelling of the distill operations.
	"MemoryDistillBrain",
	"MemoryDistillProfile",
	// The OKR rollup-by-id verb.
	"ObjectiveRollup",
	// The wiring setters the composition root replaced with the builders.
	"SetForge",
	"SetMCPRegistry",
	// The daemon's own MCP readers (the row surface answers the same rows).
	"MCPRegistration",
	"MCPRegistrations",
	// The daemon's seven forwarders over the script-tool forge's lifecycle.
	"DraftScriptTool",
	"UpdateScriptTool",
	"TestScriptTool",
	"PromoteScriptTool",
	"RequestToolPromotion",
	"QuarantineScriptTool",
	"RemoveScriptTool",
	// The daemon's eight forwarders over the workflows module's engine.
	"SaveWorkflow",
	"RestoreWorkflow",
	"SetWorkflowEnabled",
	"RemoveWorkflow",
	"RunWorkflow",
	"TestWorkflowNode",
	"DraftWorkflow",
	"RefineWorkflow",
	// The daemon's three forwarders over the reasoning module's operations.
	"Council",
	"Conduct",
	"Research",
	// The daemon's nine verbs over the MCP registry's lifecycle and rows.
	"AddMCPServer",
	"SetMCPServerEnabled",
	"AttachMCPServer",
	"DetachMCPServer",
	"RemoveMCPServer",
	"AttachEnabledMCPServers",
	"MCPAttached",
	"MCPServers",
	"MCPServer",
	// The daemon's five verbs over the standing library's lifecycle and rows.
	"AddStanding",
	"SetStandingEnabled",
	"UpdateStanding",
	"RemoveStanding",
	"StandingList",
	// The daemon's five verbs over the roster's lifecycle (W3.76y).
	"AddProfile",
	"SetProfileEnabled",
	"SetProfileRetired",
	"UpdateProfile",
	"RemoveProfile",
	// The daemon's six verbs over the steering family's lifecycle (W3.76z).
	"PauseRun",
	"ResumeRun",
	"StepRun",
	"SteerRun",
	"RunControlState",
	"InterveneRun",
	// The daemon's nine verbs over the run lifecycle (W3.76aa).
	"IsHalted",
	"Halt",
	"HaltWith",
	"Resume",
	"ResumeWith",
	"CancelRun",
	"DrainAndHalt",
	"ActiveRuns",
	"ActiveRunIDs",
	// The daemon's six wrappers over the run engine's own operations
	// (W3.76ab).
	"DescribeImages",
	"MaybeDistill",
	"MaybeForge",
	"MaybeShadowEval",
	"VerifyCompletion",
	"PublishHeuristicBypass",
	// The daemon's five verbs over the run engine's own operations
	// (W3.76ac).
	"Run",
	"RunWith",
	"RunWithRetry",
	"RunAssured",
	"CompleteAgentLifecycle",
	// The daemon's four verbs over the journal's own provenance walks
	// (W3.76ad).
	"Why",
	"Causes",
	"ParentOf",
	"Verify",
	// The daemon's three verbs over the run's own live state (W3.76ae).
	"SetupRunState",
	"CleanupRunState",
	"DeregisterRunSteer",
	// The daemon's two doors over the run's own prompt assembly (W3.76af).
	"BuildRunPrompt",
	"InjectHostEnvironment",
}

// daemonPublicVerbs is the ceiling the daemon's public method set may not
// grow over.
//
// The number is a MEASUREMENT, not a wish: W3.76k left the ceiling at 220
// (the daemon held 238 public verbs before that slice and 220 after it), and
// every slice since has taken the floor DOWN — the roster, the runner, the
// resume handshake and the objective library have each shaved verbs off the
// host. A ceiling that stays where it was while the floor moves is a ratchet
// that permits exactly what the program exists to remove.
//
// So the ceiling follows the floor: 220 -> 201 (W3.76q) -> 194 (W3.76r, the
// objective lifecycle) -> 179 (W3.76s, the task board's sixteen forwarders) ->
// 173 (W3.76t, the script forge's seven forwarders) -> 166 (W3.76u, the
// workflow engine's eight forwarders) -> 164 (W3.76v, the reasoning module's
// three forwarders) -> 156 (W3.76w, the MCP registry's seven forwarders and
// two readers) -> 152 (W3.76x, the standing library's four forwarders and one
// reader) -> 148 (W3.76y, the roster's five forwarders) -> 143 (W3.76z, the
// steering family's six forwarders) -> 135 (W3.76aa, the run lifecycle's nine
// verbs) -> 129 (W3.76ab, the run engine's six wrappers) -> 125 (W3.76ac, the
// run engine's five doors — the daemon hands the module's face over instead)
// -> 121 (W3.76ad, the journal's four provenance walks) -> 118 (W3.76ae, the
// run's three live-state verbs) -> 116 (W3.76af, the run's two prompt doors).
// The daemon holds 116 public verbs today; a new verb needs a new reason, and
// this file says so.
const daemonPublicVerbs = 116

// TestTheRetiredVerbsDoNotComeBack pins the retirement: none of the sixteen
// names is a method of *Kernel any more.
func TestTheRetiredVerbsDoNotComeBack(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range retiredVerbs {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s method again — it had no caller outside the daemon, and a verb nobody calls is dead code", name)
		}
	}
}

// TestTheDaemonKeepsItsSurfaceFromGrowing pins the ceiling: the daemon's
// public verbs may go down, never up.
func TestTheDaemonKeepsItsSurfaceFromGrowing(t *testing.T) {
	methods := daemonMethodSet(t)
	if len(methods) > daemonPublicVerbs {
		names := make([]string, 0, len(methods))
		for name := range methods {
			names = append(names, name)
		}
		t.Errorf("the daemon holds %d public verbs, want at most %d — the host's surface only shrinks: %s",
			len(methods), daemonPublicVerbs, strings.Join(names, ", "))
	}
}

// TestTheDaemonDoesNotSpeakForTheFamiliesItDoesNotOwn is the shape of the
// retirement in one place: the daemon keeps the PORT of a family it does not
// own, and it does not keep a verb per operation of that family.
//
// The memory is the knowledge module's: the daemon holds the port (k.Memory)
// and the count the app layer reads, and the six operations it used to spell
// out over that port are gone. The two distill operations keep ONE spelling
// — the door (DistillBrain / DistillProfile), not a second name beside it.
func TestTheDaemonDoesNotSpeakForTheFamiliesItDoesNotOwn(t *testing.T) {
	methods := daemonMethodSet(t)
	// The port and the faces the app layer still reads.
	for _, name := range []string{"Memory", "MemoryCount", "DistillBrain", "DistillProfile"} {
		if _, ok := methods[name]; !ok {
			t.Errorf("*Kernel lost the %s verb — the daemon keeps the port and the faces the layers above read", name)
		}
	}
	// The operations nobody asked for, and the second spelling of the ones
	// that stayed.
	for _, name := range []string{
		"MemoryActive", "MemoryProfileText", "MemorySearchScoped", "MemoryRecallScoped",
		"MemoryForget", "MemoryDistillFacts", "MemoryDistillBrain", "MemoryDistillProfile",
	} {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel speaks for the memory family again (%s) — the family is the knowledge module's, and one operation has one name", name)
		}
	}
}

// daemonMethodSet is the daemon's public method set: the exported methods
// *Kernel carries, read from the package's own sources.
func daemonMethodSet(t *testing.T) map[string]bool {
	t.Helper()
	methods := map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(".", e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() {
				continue
			}
			// Only the methods on *Kernel count: the daemon's public surface.
			if !kernelReceiver(fn.Recv) {
				continue
			}
			methods[fn.Name.Name] = true
		}
	}
	return methods
}

// kernelReceiver reports whether the method is declared on *Kernel.
func kernelReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	expr := recv.List[0].Type
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "Kernel"
}
