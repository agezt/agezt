// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"sort"
	"strings"
)

// Violation kinds. Each one is a principle from
// architecture/20-target-architecture.md §1 made mechanical.
const (
	// KindUpward: a package imports a package of a HIGHER layer (P1). This is
	// also what catches kernel → plugins and kernel → cmd edges.
	KindUpward = "upward"
	// KindCrossModule: two L3 modules import each other's internals instead
	// of the other module's api package (P5).
	KindCrossModule = "cross-module"
	// KindAdapterBypass: an inbound adapter reaches a module directly instead
	// of going through the app layer's operations (P2).
	KindAdapterBypass = "adapter-bypass"
	// KindPluginReach: a plugin depends on modules, app or adapters instead
	// of only contracts and platform (§2: plugins implement L1 contracts).
	KindPluginReach = "plugin-reach"
	// KindImpureContract: a package under the contract tree imports something
	// other than the standard library or another contract package (P4). Layer
	// order alone cannot catch this — L0 helpers and third-party modules are
	// "below" L1 — but a contract that pulls in logic is no longer a contract.
	KindImpureContract = "impure-contract"
)

// contractTree is the module-relative prefix of the pure contract packages.
const contractTree = "kernel/contract/"

// Pkg is the slice of `go list -json` output archcheck needs.
type Pkg struct {
	ImportPath string
	Imports    []string
	Dir        string
	GoFiles    []string
}

// Violation is one forbidden dependency edge between two packages.
type Violation struct {
	Kind     string
	Importer string // module-relative
	Imported string // module-relative
}

// Key is the stable allowlist line for the violation.
func (v Violation) Key() string {
	return fmt.Sprintf("%s %s -> %s", v.Kind, v.Importer, v.Imported)
}

// Report is the result of evaluating the dependency graph.
type Report struct {
	Violations []Violation
	Unmapped   []string // packages no rule classifies
	Packages   int
}

// evaluate applies every rule to every in-module import edge. Only
// non-test imports are considered: tests may reach across layers to build
// fixtures, and the architecture governs what ships in a binary.
func evaluate(cfg *Config, pkgs []Pkg) Report {
	var rep Report
	classes := map[string]Class{}
	for _, p := range pkgs {
		rel, ok := cfg.relPath(p.ImportPath)
		if !ok {
			continue
		}
		rep.Packages++
		cl, ok := cfg.classify(rel)
		if !ok {
			rep.Unmapped = append(rep.Unmapped, rel)
			continue
		}
		classes[rel] = cl
	}

	seen := map[string]bool{}
	add := func(v Violation) {
		if !seen[v.Key()] {
			seen[v.Key()] = true
			rep.Violations = append(rep.Violations, v)
		}
	}
	for _, p := range pkgs {
		from, ok := cfg.relPath(p.ImportPath)
		if !ok || !strings.HasPrefix(from, contractTree) {
			continue
		}
		for _, imp := range p.Imports {
			if isStdlib(imp) {
				continue
			}
			if to, ok := cfg.relPath(imp); ok && strings.HasPrefix(to, contractTree) {
				continue
			}
			to, ok := cfg.relPath(imp)
			if !ok {
				to = imp // third-party: report the full path
			}
			add(Violation{Kind: KindImpureContract, Importer: from, Imported: to})
		}
	}
	for _, p := range pkgs {
		from, ok := cfg.relPath(p.ImportPath)
		if !ok {
			continue
		}
		fc, ok := classes[from]
		if !ok {
			continue
		}
		for _, imp := range p.Imports {
			to, ok := cfg.relPath(imp)
			if !ok {
				continue
			}
			tc, ok := classes[to]
			if !ok {
				continue // reported as unmapped when the package itself is listed
			}
			kind := edgeKind(fc, tc, to)
			if kind == "" {
				continue
			}
			add(Violation{Kind: kind, Importer: from, Imported: to})
		}
	}
	sort.Slice(rep.Violations, func(i, j int) bool { return rep.Violations[i].Key() < rep.Violations[j].Key() })
	sort.Strings(rep.Unmapped)
	return rep
}

// edgeKind returns the violation kind of the edge from → to, or "" when the
// edge is allowed. Precedence: upward first (the most fundamental breach),
// then the layer-specific rules.
func edgeKind(from, to Class, toRel string) string {
	if from.Layer == LayerRoot {
		return "" // binaries, SDKs and tooling compose everything
	}
	if to.Layer > from.Layer {
		return KindUpward
	}
	switch from.Layer {
	case LayerModule:
		if to.Layer == LayerModule && to.Module != from.Module && !isAPIPackage(toRel) {
			return KindCrossModule
		}
	case LayerAdapter:
		if to.Layer == LayerModule {
			return KindAdapterBypass
		}
	case LayerPlugin:
		if to.Layer == LayerModule || to.Layer == LayerApp || to.Layer == LayerAdapter {
			return KindPluginReach
		}
	}
	return ""
}

// isStdlib reports whether an import path names a standard-library package:
// the Go toolchain reserves dot-free first path elements for it.
func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}
