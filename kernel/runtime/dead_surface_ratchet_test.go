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
}

// daemonPublicVerbs is the ceiling W3.76k left: the daemon's public method
// set may shrink from here, but it must not grow back over it.
//
// The number is a MEASUREMENT, not a wish: the daemon held 238 public verbs
// before this slice and 220 after it (eighteen verbs retired — sixteen with
// no caller outside the daemon, two more that were a second spelling of an
// operation the daemon already had a door for). The ceiling is the floor it
// reached — a new verb needs a new reason, and this file says so.
const daemonPublicVerbs = 220

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
