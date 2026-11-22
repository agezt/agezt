// SPDX-License-Identifier: MIT

package main

// The top-level command vocabulary has one home: cmd/agt/commands_top.go.
//
// The CLI used to spell its top-level names twice — cmd_register.go declared
// the commands (name, aliases, runner) and help_data.go declared the same
// names again with their summaries and usage lines — and the root dispatcher
// switched on args[0] a third time to answer "version" and "help" before the
// registry ever saw them. Two tests kept the two copies in step, which is a
// guard, not a single source.
//
// The table is now the only place a top-level name is written: the registry
// and the help overview are derived from it. The tests below fail when a
// second copy appears, when the root dispatcher starts keeping a list of its
// own again, when a name is registered twice, and when a command loses its
// runner, its summary or its usage block.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// the table
// ---------------------------------------------------------------------------

// topEntry is one top-level command, read out of the table.
type topEntry struct {
	name    string
	aliases []string
	group   string
	summary string
	detail  int    // the number of usage lines
	run     string // the runner, "" when the command has none
}

// topTable reads the top-level vocabulary out of commands_top.go.
func topTable(t *testing.T) []topEntry {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "commands_top.go", nil, 0)
	if err != nil {
		t.Fatalf("parse the top-level vocabulary: %v", err)
	}
	entries := []topEntry{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			lhs, ok := assign.Lhs[0].(*ast.Ident)
			if !ok || lhs.Name != "cliTopCommands" {
				return true
			}
			lit, ok := assign.Rhs[0].(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, item := range lit.Elts {
				command, ok := item.(*ast.CompositeLit)
				if !ok {
					continue
				}
				entry := topEntry{}
				for _, elt := range command.Elts {
					field, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := field.Key.(*ast.Ident)
					if !ok {
						continue
					}
					switch key.Name {
					case "name":
						if v, ok := field.Value.(*ast.BasicLit); ok {
							entry.name = unquote(v.Value)
						}
					case "aliases":
						list, ok := field.Value.(*ast.CompositeLit)
						if !ok {
							continue
						}
						for _, alias := range list.Elts {
							if v, ok := alias.(*ast.BasicLit); ok {
								entry.aliases = append(entry.aliases, unquote(v.Value))
							}
						}
					case "group":
						if v, ok := field.Value.(*ast.BasicLit); ok {
							entry.group = unquote(v.Value)
						}
					case "summary":
						if v, ok := field.Value.(*ast.BasicLit); ok {
							entry.summary = unquote(v.Value)
						}
					case "detail":
						list, ok := field.Value.(*ast.CompositeLit)
						if !ok {
							continue
						}
						entry.detail = len(list.Elts)
					case "run":
						entry.run = runnerName(field.Value)
					}
				}
				entries = append(entries, entry)
			}
			return true
		})
	}
	if len(entries) == 0 {
		t.Fatal("the top-level vocabulary is empty — the command names lost their home")
	}
	return entries
}

// unquote reads the value of a string literal.
func unquote(literal string) string {
	value, err := strconv.Unquote(literal)
	if err != nil {
		return strings.Trim(literal, `"`)
	}
	return value
}

// runnerName names the runner of a command: the function it calls, or
// "closure" when the runner is a function literal.
func runnerName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	case *ast.FuncLit:
		return "closure"
	}
	return ""
}

// ---------------------------------------------------------------------------
// the tests
// ---------------------------------------------------------------------------

// TestTopLevelVocabularyHasOneHome proves that no second copy of the
// top-level names can appear: no file registers a command outside the table,
// and no file switches on args[0] to answer a top-level name by hand.
func TestTopLevelVocabularyHasOneHome(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd/agt: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if entry.Name() == "commands_top.go" {
			continue
		}
		parsed, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				id, ok := node.Fun.(*ast.Ident)
				if !ok || id.Name != "Register" {
					return true
				}
				t.Errorf("%s registers a command outside the top-level vocabulary table; "+
					"the table (commands_top.go) is the only place a top-level name is written",
					entry.Name())
			case *ast.SwitchStmt:
				if !switchesOnArgsZero(node) {
					return true
				}
				t.Errorf("%s switches on args[0]; the top-level names live in the "+
					"vocabulary table, the dispatcher only looks them up", entry.Name())
			}
			return true
		})
	}
}

// switchesOnArgsZero reports whether a switch discriminates on args[0].
func switchesOnArgsZero(node *ast.SwitchStmt) bool {
	tag, ok := node.Tag.(*ast.IndexExpr)
	if !ok {
		return false
	}
	index, ok := tag.Index.(*ast.BasicLit)
	if !ok {
		return false
	}
	return index.Value == "0"
}

// TestTopLevelNamesAreUnique proves that no two commands answer to the same
// name: the canonical names are unique, and so are the aliases — an alias
// that shadows another command's name would silently win.
func TestTopLevelNamesAreUnique(t *testing.T) {
	owners := map[string]string{}
	for _, entry := range topTable(t) {
		names := append([]string{entry.name}, entry.aliases...)
		for _, name := range names {
			if owner, taken := owners[name]; taken {
				t.Errorf("the name %q is answered by two commands: %q and %q", name, owner, entry.name)
				continue
			}
			owners[name] = entry.name
		}
	}
}

// TestEveryTopLevelCommandIsComplete proves that every command of the table
// can be run and can be explained: it has a runner, a summary, a usage block
// and a group the help overview knows.
func TestEveryTopLevelCommandIsComplete(t *testing.T) {
	groups := map[string]bool{}
	for _, group := range cliTopGroups {
		groups[group] = true
	}
	for _, entry := range topTable(t) {
		switch {
		case entry.name == "":
			t.Error("a command of the table has no name")
		case entry.run == "":
			t.Errorf("%q has no runner", entry.name)
		case entry.summary == "":
			t.Errorf("%q has no summary for the help overview", entry.name)
		case entry.detail == 0:
			t.Errorf("%q has no usage block; `agt help %s` would be empty", entry.name, entry.name)
		case entry.group == "":
			t.Errorf("%q is in no group; the help overview lists it nowhere", entry.name)
		case !groups[entry.group]:
			t.Errorf("%q is in the group %q, which the help overview does not list", entry.name, entry.group)
		}
	}
}

// TestTheRegistryIsTheTable proves that the registry holds the table and
// nothing else: every command of the table is registered under its name and
// its aliases, and no other name is registered.
func TestTheRegistryIsTheTable(t *testing.T) {
	entries := topTable(t)
	expected := map[string]bool{}
	for _, entry := range entries {
		expected[entry.name] = true
		for _, alias := range entry.aliases {
			expected[alias] = true
		}
	}
	for name := range expected {
		if _, ok := CommandRegistry[name]; !ok {
			t.Errorf("%q is in the vocabulary table but not registered", name)
		}
	}
	for name := range CommandRegistry {
		if !expected[name] {
			t.Errorf("%q is registered but is not in the vocabulary table", name)
		}
	}
}

// TestHelpOverviewIsDerivedFromTheTable proves that the help overview is the
// table read another way: every command of the table is listed exactly once,
// with the summary and the usage block the table carries, and no other
// command is listed.
func TestHelpOverviewIsDerivedFromTheTable(t *testing.T) {
	entries := topTable(t)
	byName := map[string]topEntry{}
	for _, entry := range entries {
		byName[entry.name] = entry
	}
	listed := map[string]int{}
	groups := helpGroups()
	if len(groups) != len(cliTopGroups) {
		t.Errorf("the help overview has %d groups, the table has %d", len(groups), len(cliTopGroups))
	}
	for i, group := range groups {
		if i < len(cliTopGroups) && group.title != cliTopGroups[i] {
			t.Errorf("help group %d is %q, the table's order says %q", i, group.title, cliTopGroups[i])
		}
		for _, command := range group.commands {
			listed[command.name]++
			entry, ok := byName[command.name]
			if !ok {
				t.Errorf("the help overview lists %q, which the table does not have", command.name)
				continue
			}
			if command.summary != entry.summary {
				t.Errorf("%q: the help summary %q is not the table's %q", command.name, command.summary, entry.summary)
			}
			if len(command.detail) != entry.detail {
				t.Errorf("%q: the help block has %d lines, the table has %d", command.name, len(command.detail), entry.detail)
			}
		}
	}
	for _, entry := range entries {
		switch listed[entry.name] {
		case 0:
			t.Errorf("%q is in the vocabulary table but the help overview does not list it", entry.name)
		case 1:
		default:
			t.Errorf("%q is listed in the help overview %d times", entry.name, listed[entry.name])
		}
	}
}

// TestTheRootDispatcherKeepsNoList proves that the root dispatch answers
// every top-level name through the registry: it does not switch on the first
// argument by hand.
func TestTheRootDispatcherKeepsNoList(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "run" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			node, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			if switchesOnArgsZero(node) {
				t.Error("the root dispatch switches on args[0]; the registry answers every top-level name")
			}
			return true
		})
	}
}

// TestTheHelpOverviewGroupsAreOrdered proves that the group order is written
// down once: the table's groups are listed in the order the help prints them.
func TestTheHelpOverviewGroupsAreOrdered(t *testing.T) {
	groups := helpGroups()
	if len(groups) != len(cliTopGroups) {
		t.Fatalf("the help overview has %d groups, the table's order has %d", len(groups), len(cliTopGroups))
	}
	for i, group := range groups {
		if group.title != cliTopGroups[i] {
			t.Errorf("help group %d is %q, the table's order says %q", i, group.title, cliTopGroups[i])
		}
	}
}
