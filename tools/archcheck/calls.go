// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Forbidden-call rules (architecture/20-target-architecture.md principles P3
// and P7: one way to do each infrastructural thing). Each rule names calls
// that bypass a platform guarantee when made from an arbitrary package:
//
//   - exec: starting a child process without the sandbox launcher means no
//     env scrubbing (secrets leak into children), no isolation, no limits.
//   - http-client: a hand-built or default HTTP client means no SSRF guard,
//     no uniform retry/Retry-After, no body cap.
//   - raw-write: os.WriteFile/os.Create skip the atomic writer (fsync +
//     unique temp + rename) and the store framework's permissions.
//
// The rule SET is fixed here; layers.json only says WHERE each rule's calls
// are legitimate (the platform package that owns the guarantee).
const (
	CallExec       = "exec"
	CallHTTPClient = "http-client"
	CallRawWrite   = "raw-write"
)

// callRules maps rule → import path → selector names that violate it.
var callRules = map[string]map[string][]string{
	CallExec:       {"os/exec": {"Command", "CommandContext"}},
	CallHTTPClient: {"net/http": {"DefaultClient", "Get", "Post", "Head", "PostForm"}},
	CallRawWrite:   {"os": {"WriteFile", "Create"}},
}

// constructRules are TYPES whose construction (T{...} or new(T)) violates a
// rule, while merely referring to them does not: a field of type
// *http.Client that a constructor injects is exactly how a guarded client
// should arrive.
var constructRules = map[string]map[string][]string{
	CallHTTPClient: {"net/http": {"Client"}},
}

// CallPolicy is one entry of layers.json "calls": the packages where the
// rule's calls are allowed. Packages in layers 0 and 7 are always exempt:
// foundation helpers ARE the implementations, and binaries/tooling compose.
type CallPolicy struct {
	Rule  string   `json:"rule"`
	Allow []string `json:"allow"`
}

func validateCallPolicies(policies []CallPolicy) error {
	seen := map[string]bool{}
	for _, p := range policies {
		if _, ok := callRules[p.Rule]; !ok {
			return fmt.Errorf("calls: unknown rule %q", p.Rule)
		}
		if seen[p.Rule] {
			return fmt.Errorf("calls: duplicate rule %q", p.Rule)
		}
		seen[p.Rule] = true
	}
	for rule := range callRules {
		if !seen[rule] {
			return fmt.Errorf("calls: rule %q has no policy entry (use an empty allow list to allow nowhere)", rule)
		}
	}
	return nil
}

func (c *Config) callAllowed(rule, rel string) bool {
	for _, p := range c.Calls {
		if p.Rule != rule {
			continue
		}
		for _, pat := range p.Allow {
			if _, ok := match(pat, rel); ok {
				return true
			}
		}
	}
	return false
}

// CallCount is the number of forbidden call sites of one rule in one package.
type CallCount struct {
	Rule string
	Pkg  string // module-relative
	N    int
}

// Key is the allowlist identity (without the count).
func (c CallCount) Key() string { return c.Rule + " " + c.Pkg }

// scanCalls counts forbidden call sites in every in-scope package. Only
// non-test files are parsed (Pkg.GoFiles excludes _test.go).
func scanCalls(cfg *Config, pkgs []Pkg) ([]CallCount, error) {
	var out []CallCount
	for _, p := range pkgs {
		rel, ok := cfg.relPath(p.ImportPath)
		if !ok {
			continue
		}
		cl, ok := cfg.classify(rel)
		if !ok || cl.Layer == LayerFoundation || cl.Layer == LayerRoot {
			continue
		}
		counts := map[string]int{}
		for _, f := range p.GoFiles {
			if err := countFileCalls(filepath.Join(p.Dir, f), counts); err != nil {
				return nil, err
			}
		}
		for rule, n := range counts {
			if n > 0 && !cfg.callAllowed(rule, rel) {
				out = append(out, CallCount{Rule: rule, Pkg: rel, N: n})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out, nil
}

func countFileCalls(path string, counts map[string]int) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	// local import name → import path, for the paths any rule cares about.
	names := map[string]string{}
	for _, imp := range file.Imports {
		ip, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		watched := false
		for _, table := range []map[string]map[string][]string{callRules, constructRules} {
			for _, byPath := range table {
				if _, ok := byPath[ip]; ok {
					watched = true
				}
			}
		}
		if !watched {
			continue
		}
		name := ip[strings.LastIndex(ip, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		names[name] = ip
	}
	if len(names) == 0 {
		return nil
	}
	// resolve returns the import path and selector of pkg.Name, if pkg is a
	// watched import.
	resolve := func(e ast.Expr) (string, string, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return "", "", false
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return "", "", false
		}
		ip, ok := names[id.Name]
		return ip, sel.Sel.Name, ok
	}
	hit := func(table map[string]map[string][]string, ip, name string) {
		for rule, byPath := range table {
			for _, s := range byPath[ip] {
				if name == s {
					counts[rule]++
				}
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if ip, name, ok := resolve(x); ok {
				hit(callRules, ip, name)
			}
		case *ast.CompositeLit:
			if ip, name, ok := resolve(x.Type); ok {
				hit(constructRules, ip, name)
			}
		case *ast.CallExpr:
			if fn, ok := x.Fun.(*ast.Ident); ok && fn.Name == "new" && len(x.Args) == 1 {
				if ip, name, ok := resolve(x.Args[0]); ok {
					hit(constructRules, ip, name)
				}
			}
		}
		return true
	})
	return nil
}

// callDiff compares found counts with the count allowlist. A count above the
// allowlisted one is new debt; a count BELOW it is stale (the allowlist must
// be lowered so the fixed sites cannot come back).
type callDiff struct {
	current      map[string]int // key → found
	stillAllowed map[string]int // key → min(found, allowed)
	added        []string       // human lines for new debt
	stale        []string       // human lines for stale entries
}

func compareCalls(found []CallCount, allowed map[string]int) callDiff {
	d := callDiff{current: map[string]int{}, stillAllowed: map[string]int{}}
	seen := map[string]bool{}
	for _, c := range found {
		k := c.Key()
		seen[k] = true
		d.current[k] = c.N
		a := allowed[k]
		switch {
		case c.N > a:
			d.added = append(d.added, fmt.Sprintf("%s: %d call site(s), %d allowlisted", k, c.N, a))
			if a > 0 {
				d.stillAllowed[k] = a
			}
		case c.N < a:
			d.stale = append(d.stale, fmt.Sprintf("%s: allowlist says %d, now %d", k, a, c.N))
			d.stillAllowed[k] = c.N
		default:
			d.stillAllowed[k] = a
		}
	}
	for k, a := range allowed {
		if !seen[k] {
			d.stale = append(d.stale, fmt.Sprintf("%s: allowlist says %d, now 0", k, a))
		}
	}
	sort.Strings(d.added)
	sort.Strings(d.stale)
	return d
}
