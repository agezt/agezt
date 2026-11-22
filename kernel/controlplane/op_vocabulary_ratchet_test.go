// SPDX-License-Identifier: MIT

package controlplane

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The operation vocabulary has one home: kernel/contract/opapi/names*.go.
// Every operation the daemon answers to is declared there, exactly once, and
// the application layer binds that constant into the operation's spec.
//
// Two things can break that, and both are silent at runtime:
//
//   - an operation registered under a name the catalogue does not carry — the
//     daemon answers to a name nothing else knows about;
//   - a catalogue name with no operation behind it — a name an operator can
//     type that the daemon will not answer to.
//
// The tests below fail on both.

// nativeCommands are the names the control plane answers to without a typed
// operation behind them. `whoami` is native by design: it echoes the
// transport principal and must not route the tenant it names.
var nativeCommands = map[string]string{
	"whoami": "echoes the transport principal; native by design",
}

// catalogue reads the canonical operation names out of the contract package.
// The constants are the source of truth and Go cannot enumerate them, so the
// test reads the declarations themselves: a name that is declared twice, or
// declared with two values, is a defect the test must see.
func catalogue(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join("..", "contract", "opapi")
	files, err := filepath.Glob(filepath.Join(dir, "names*.go"))
	if err != nil {
		t.Fatalf("glob catalogue: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no catalogue files in %s — the operation vocabulary lost its home", dir)
	}
	names := map[string]string{}
	values := map[string]string{}
	for _, file := range files {
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					lit, ok := value.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if previous, seen := values[name.Name]; seen {
						t.Errorf("catalogue declares %s twice (%q and %q)", name.Name, previous, lit.Value)
					}
					values[name.Name] = lit.Value
					names[strings.Trim(lit.Value, `"`)] = name.Name
				}
			}
		}
	}
	return names
}

// TestOperationVocabularyMatchesCatalogue proves the registry and the
// catalogue are the same vocabulary.
func TestOperationVocabularyMatchesCatalogue(t *testing.T) {
	catalogue := catalogue(t)

	registered := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		name := operation.Spec().Name
		if name == "" {
			t.Error("an operation registered without a name")
			continue
		}
		if _, known := catalogue[name]; !known {
			t.Errorf("operation %q is registered but the catalogue does not carry it", name)
		}
		if registered[name] {
			t.Errorf("operation %q is registered twice", name)
		}
		registered[name] = true
	}

	for value, identifier := range catalogue {
		if registered[value] {
			continue
		}
		if reason, native := nativeCommands[value]; native {
			t.Logf("catalogue name %q (%s) is native: %s", value, identifier, reason)
			continue
		}
		t.Errorf("catalogue name %q (%s) has no operation behind it", value, identifier)
	}
}

// nameLiteral finds an operation name written as a string literal where the
// catalogue's constant belongs. The literal has the same value, so nothing
// breaks at runtime — but the catalogue stops being the source of truth the
// moment a name is written in two places.
var nameLiteral = regexp.MustCompile(`Name:\s*"([a-z0-9_.\-]+)"`)

// TestApplicationLayerNamesComeFromTheCatalogue proves the application layer
// binds the catalogue's constants instead of repeating the names.
func TestApplicationLayerNamesComeFromTheCatalogue(t *testing.T) {
	catalogue := catalogue(t)

	root := filepath.Join("..", "app")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range nameLiteral.FindAllStringSubmatch(string(source), -1) {
			if _, known := catalogue[match[1]]; known {
				t.Errorf("%s writes the operation name %q as a literal; the catalogue's constant belongs here",
					filepath.ToSlash(path), match[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

// TestNativeCommandsAreDeclared keeps the native exception honest: a name that
// is declared native must actually be a native command, and a native command
// must be declared.
func TestNativeCommandsAreDeclared(t *testing.T) {
	catalogue := catalogue(t)
	for value := range nativeCommands {
		if _, known := catalogue[value]; !known {
			t.Errorf("native command %q is not in the catalogue", value)
		}
	}
	registered := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		registered[operation.Spec().Name] = true
	}
	names := make([]string, 0, len(nativeCommands))
	for name := range nativeCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if registered[name] {
			t.Errorf("%q is declared native but has a typed operation behind it", name)
		}
	}
}
