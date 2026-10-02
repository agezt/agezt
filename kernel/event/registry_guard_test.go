// SPDX-License-Identifier: MIT

package event

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// reservedKinds are declared with no emitter yet, each with a recorded reason.
var reservedKinds = map[string]string{}

// productionGoFiles returns the module's non-test .go files outside this package.
func productionGoFiles(t *testing.T) map[string][]byte {
	t.Helper()
	root := filepath.Join("..", "..")
	out := map[string][]byte{}
	for _, top := range []string{"cmd", "kernel", "plugins", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if n := d.Name(); n == "testdata" || n == "node_modules" || strings.HasPrefix(n, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			if filepath.Base(filepath.Dir(p)) == "event" && filepath.Base(filepath.Dir(filepath.Dir(p))) == "kernel" {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[p] = b
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestKindRegistryIsClosed: every event kind the system emits is a declared
// constant, and every declared constant is emitted or consumed somewhere.
//
// Six kinds — including the security-audit policy.auto_approved and
// prompt_injection.warned — used to be built from string literals with
// event.Kind("…"), invisible to anything reading this registry, while seven
// declared kinds had never been emitted at all (W1.6).
func TestKindRegistryIsClosed(t *testing.T) {
	files := productionGoFiles(t)
	if len(files) < 100 {
		t.Fatalf("found only %d production files; is the walk rooted correctly?", len(files))
	}

	// 1. No kind is minted from a string literal outside this package.
	fset := token.NewFileSet()
	for path, src := range files {
		if !strings.Contains(string(src), "event.Kind(") {
			continue
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Kind" {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "event" {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				v, _ := strconv.Unquote(lit.Value)
				t.Errorf("%s: event.Kind(%q) — declare it as a constant in kernel/event/kinds.go", fset.Position(call.Pos()), v)
			}
			return true
		})
	}

	// 2. Every declared kind is referenced by production code, or reserved.
	src, err := os.ReadFile("kinds.go")
	if err != nil {
		t.Fatal(err)
	}
	decl := regexp.MustCompile(`(?m)^\s*(Kind\w+)\s+Kind\s*=`)
	used := map[string]bool{}
	ref := regexp.MustCompile(`\bevent\.(Kind\w+)\b`)
	for _, b := range files {
		for _, m := range ref.FindAllSubmatch(b, -1) {
			used[string(m[1])] = true
		}
	}
	for _, m := range decl.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if _, ok := reservedKinds[name]; ok {
			continue
		}
		if !used[name] {
			t.Errorf("%s is declared but never emitted or consumed — delete it, or add it to reservedKinds with a reason", name)
		}
	}
}
