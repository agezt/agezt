// SPDX-License-Identifier: MIT

package main

// The command vocabulary has one home: cmd/agt/commands_dict.go.
//
// The CLI used to spell the same list of sub-command names four or five
// times per domain — in the dispatcher's switch, in the usage line, in the
// unknown-sub-command message, in the "subcommand required" guard and in the
// help table. The copies disagreed with each other often enough to matter:
// `agt agent` printed a usage line that was missing `authority`, and the
// help table still carried lists the switch had outgrown.
//
// The table is now the only place a sub-command name is written. The tests
// below fail when a second copy appears, when a command's operations no
// longer match the handler that runs it, and when a dispatcher starts
// keeping a list of its own again.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// the table
// ---------------------------------------------------------------------------

// dictCommand is one command of one domain, read out of the table.
type dictCommand struct {
	domain  string
	names   []string
	ops     []string
	runner  []string        // the package functions the runner calls
	handler string          // the handler the runner runs, "" for a closure
	own     map[string]bool // the operations the runner expression writes itself
}

// dictDomain is one domain of the table.
type dictDomain struct {
	name     string
	commands []dictCommand
}

// dictionary reads the table out of cmd/agt/commands_dict.go.
func dictionary(t *testing.T) []dictDomain {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "commands_dict.go", nil, 0)
	if err != nil {
		t.Fatalf("parse the command dictionary: %v", err)
	}
	domains := []dictDomain{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// The domains are filled by fillDomains (W3.81 made the order
		// explicit) or by init (before that).
		if fn.Name.Name != "fillDomains" && fn.Name.Name != "init" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			lhs, ok := assign.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			lit, ok := assign.Rhs[0].(*ast.CompositeLit)
			if !ok {
				return true
			}
			if !isDomainType(lit.Type) {
				// another composite literal inside a command's body
				return true
			}
			domain := dictDomain{name: lhs.Name}
			for _, elt := range lit.Elts {
				field, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := field.Key.(*ast.Ident)
				if !ok || key.Name != "commands" {
					continue
				}
				list, ok := field.Value.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, item := range list.Elts {
					entry, ok := item.(*ast.CompositeLit)
					if !ok {
						continue
					}
					domain.commands = append(domain.commands, readCommand(t, domain.name, entry))
				}
			}
			domains = append(domains, domain)
			return true
		})
	}
	if len(domains) == 0 {
		t.Fatal("the command dictionary is empty — the command vocabulary lost its home")
	}
	return domains
}

// isDomainType reports whether a composite literal is a domain.
func isDomainType(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name == "cliDomain"
	case *ast.SelectorExpr:
		return x.Sel.Name == "cliDomain"
	}
	return false
}

// readCommand reads one command out of the table.
func readCommand(t *testing.T, domain string, entry *ast.CompositeLit) dictCommand {
	t.Helper()
	command := dictCommand{domain: domain}
	for _, elt := range entry.Elts {
		field, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := field.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "names":
			list, ok := field.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, item := range list.Elts {
				lit, ok := item.(*ast.BasicLit)
				if !ok {
					continue
				}
				command.names = append(command.names, strings.Trim(lit.Value, `"`))
			}
		case "ops":
			list, ok := field.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, item := range list.Elts {
				sel, ok := item.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "opapi" {
					t.Errorf("%s: the operation %v is not a reference to the catalogue's constant",
						domain, item)
					continue
				}
				command.ops = append(command.ops, sel.Sel.Name)
			}
		case "run":
			command.runner = calledFunctions(field.Value)
			command.handler = handlerOf(field.Value)
			command.own = ownOperations(field.Value)
		}
	}
	return command
}

// calledFunctions lists the package functions a runner expression calls.
func calledFunctions(expr ast.Expr) []string {
	out := []string{}
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok {
			out = append(out, id.Name)
		}
		for _, arg := range call.Args {
			if id, ok := arg.(*ast.Ident); ok {
				out = append(out, id.Name)
			}
		}
		return true
	})
	return out
}

// handlerOf returns the handler a runner expression runs: the function a
// shifted()/direct() wrapper carries, or "" when the runner is a closure.
func handlerOf(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		// a closure
		return ""
	}
	if id.Name != "shifted" && id.Name != "direct" {
		return ""
	}
	for _, arg := range call.Args {
		if handler, ok := arg.(*ast.Ident); ok {
			return handler.Name
		}
	}
	return ""
}

// ownOperations returns the catalogue operations a runner expression writes
// itself.
func ownOperations(expr ast.Expr) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(expr, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "opapi" {
			out[sel.Sel.Name] = true
		}
		return true
	})
	return out
}

// ---------------------------------------------------------------------------
// the package under test
// ---------------------------------------------------------------------------

// packageFacts is what the test knows about the cmd/agt package.
type packageFacts struct {
	// ops is the operation each function reaches.
	ops map[string]map[string]bool
	// dispatchers maps a domain variable to the functions that look it up.
	dispatchers map[string][]string
	// switches lists the functions that still switch on args[0].
	switches map[string]bool
}

// facts reads the cmd/agt package.
func facts(t *testing.T, domains []dictDomain) *packageFacts {
	t.Helper()
	fset := token.NewFileSet()
	files := []*ast.File{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd/agt: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		files = append(files, parsed)
	}

	// the functions a domain runs: a dispatcher that looks a domain up runs
	// everything that domain carries.
	table := map[string]map[string]bool{}
	for _, domain := range domains {
		runners := map[string]bool{}
		for _, command := range domain.commands {
			for _, runner := range command.runner {
				if runner == "shifted" || runner == "direct" {
					continue
				}
				runners[runner] = true
			}
		}
		table[domain.name] = runners
	}

	calls := map[string]map[string]bool{}
	own := map[string]map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			called := map[string]bool{}
			operations := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if id, ok := x.Fun.(*ast.Ident); ok {
						called[id.Name] = true
					}
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
						if recv, ok := sel.X.(*ast.Ident); ok && sel.Sel.Name == "lookup" {
							for runner := range table[recv.Name] {
								called[runner] = true
							}
						}
					}
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && id.Name == "opapi" {
						operations[x.Sel.Name] = true
					}
				}
				return true
			})
			calls[fn.Name.Name] = called
			own[fn.Name.Name] = operations
		}
	}

	out := &packageFacts{
		ops:         map[string]map[string]bool{},
		dispatchers: map[string][]string{},
		switches:    map[string]bool{},
	}
	var reachable func(name string, seen map[string]bool) map[string]bool
	reachable = func(name string, seen map[string]bool) map[string]bool {
		if seen[name] {
			return map[string]bool{}
		}
		seen[name] = true
		result := map[string]bool{}
		for op := range own[name] {
			result[op] = true
		}
		for call := range calls[name] {
			if call == name {
				continue
			}
			if _, known := calls[call]; !known {
				continue
			}
			for op := range reachable(call, seen) {
				result[op] = true
			}
		}
		return result
	}
	for name := range calls {
		out.ops[name] = reachable(name, map[string]bool{})
	}

	// the functions that look a domain up, and the functions that still
	// switch on the sub-command name.
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					sel, ok := x.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					recv, ok := sel.X.(*ast.Ident)
					if !ok || !strings.HasPrefix(recv.Name, "cli") {
						return true
					}
					if sel.Sel.Name != "lookup" {
						return true
					}
					out.dispatchers[recv.Name] = append(out.dispatchers[recv.Name], fn.Name.Name)
				case *ast.SwitchStmt:
					if isArgsIndex(x.Tag, 0) {
						out.switches[fn.Name.Name] = true
					}
				}
				return true
			})
		}
	}
	return out
}

// isArgsIndex reports whether an expression is args[N].
func isArgsIndex(expr ast.Expr, index int) bool {
	ie, ok := expr.(*ast.IndexExpr)
	if !ok {
		return false
	}
	id, ok := ie.X.(*ast.Ident)
	if !ok || id.Name != "args" {
		return false
	}
	lit, ok := ie.Index.(*ast.BasicLit)
	if !ok {
		return false
	}
	return lit.Value == itoa(index)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// ---------------------------------------------------------------------------
// the tests
// ---------------------------------------------------------------------------

// domainOwnsAll reports whether every name of a group is a name of a domain.
func domainOwnsAll(domain dictDomain, group []string) bool {
	owned := map[string]bool{}
	for _, command := range domain.commands {
		for _, name := range command.names {
			owned[name] = true
		}
	}
	for _, name := range group {
		if !owned[name] {
			return false
		}
	}
	return len(group) > 1
}

// TestDictionaryIsTheOnlyCommandVocabulary proves that no message spells a
// domain's command list out by hand any more. A literal that carries two or
// more names of one domain is a second copy of that domain's vocabulary.
func TestDictionaryIsTheOnlyCommandVocabulary(t *testing.T) {
	domains := dictionary(t)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd/agt: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if entry.Name() == "commands_dict.go" {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, literal := range stringLiterals(string(source)) {
			for _, group := range listGroups(literal) {
				owners := []string{}
				for _, domain := range domains {
					if domainOwnsAll(domain, group) {
						owners = append(owners, domain.name)
					}
				}
				if len(owners) == 0 {
					continue
				}
				t.Errorf("%s writes the command list %q of %s out by hand; the dictionary derives it",
					entry.Name(), strings.Join(group, "|"), strings.Join(owners, ", "))
			}
		}
	}
}

// TestDictionaryOperationsMatchTheirHandlers proves that the operations a
// command claims are the operations its handler really reaches. A command
// that claims an operation it never calls is lying; an operation the handler
// calls that no command of that handler claims is undeclared.
func TestDictionaryOperationsMatchTheirHandlers(t *testing.T) {
	domains := dictionary(t)
	packageFacts := facts(t, domains)

	claimed := map[string]map[string]bool{}
	for _, domain := range domains {
		for _, command := range domain.commands {
			targets := []string{}
			for _, runner := range command.runner {
				if runner == "shifted" || runner == "direct" {
					continue
				}
				targets = append(targets, runner)
			}
			if len(targets) == 0 {
				t.Errorf("%s %v runs nothing", domain.name, command.names)
				continue
			}
			reachable := map[string]bool{}
			for _, target := range targets {
				ops, known := packageFacts.ops[target]
				if !known {
					// not a package function: a parameter or a local
					continue
				}
				for op := range ops {
					reachable[op] = true
				}
			}
			for _, op := range command.ops {
				if !reachable[op] && !command.own[op] {
					t.Errorf("%s %v claims the operation %q, but %v never reaches it",
						domain.name, command.names, op, targets)
				}
			}
			for _, target := range targets {
				if claimed[target] == nil {
					claimed[target] = map[string]bool{}
				}
				for _, op := range command.ops {
					claimed[target][op] = true
				}
			}
		}
	}
	for target, ops := range packageFacts.ops {
		if len(ops) == 0 || claimed[target] == nil {
			continue
		}
		for op := range ops {
			if !claimed[target][op] {
				t.Errorf("the operation %q is reachable from %s but no command that runs it claims it", op, target)
			}
		}
	}
}

// TestEveryDomainDispatchesThroughTheDictionary proves that a dispatcher does
// not keep a list of its own. The dispatchers used to switch on args[0]; now
// they look the table up.
func TestEveryDomainDispatchesThroughTheDictionary(t *testing.T) {
	domains := dictionary(t)
	packageFacts := facts(t, domains)

	for _, domain := range domains {
		dispatchers := packageFacts.dispatchers[domain.name]
		if len(dispatchers) == 0 {
			t.Errorf("the domain %s is never looked up; its commands cannot run", domain.name)
			continue
		}
		for _, dispatcher := range dispatchers {
			if packageFacts.switches[dispatcher] {
				t.Errorf("%s still switches on the sub-command name; the dictionary owns that list", dispatcher)
			}
		}
	}
}

// TestDictionaryNamesAreUnique proves that a domain cannot answer to the
// same name twice, and that every command has a name and a runner.
func TestDictionaryNamesAreUnique(t *testing.T) {
	domains := dictionary(t)
	for _, domain := range domains {
		seen := map[string]bool{}
		for _, command := range domain.commands {
			if len(command.names) == 0 {
				t.Errorf("%s has a command without a name", domain.name)
			}
			for _, name := range command.names {
				if seen[name] {
					t.Errorf("%s answers to %q twice (the command carries %v)",
						domain.name, name, sortedNames(command.names))
				}
				seen[name] = true
			}
			if len(command.runner) == 0 {
				t.Errorf("%s %v has no runner", domain.name, command.names)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// stringLiterals returns the string literals of a source file.
func stringLiterals(source string) []string {
	out := []string{}
	for _, match := range regexp.MustCompile(`"([^"\\\n]*(?:\\.[^"\\\n]*)*)"`).FindAllStringSubmatch(source, -1) {
		out = append(out, match[1])
	}
	return out
}

// listGroups returns the <a|b|c> and (a, b, c) groups of a literal.
func listGroups(literal string) [][]string {
	out := [][]string{}
	for i := 0; i < len(literal); i++ {
		switch literal[i] {
		case '<':
			end := strings.IndexByte(literal[i:], '>')
			if end < 0 {
				continue
			}
			inner := literal[i+1 : i+end]
			names := strings.Split(inner, "|")
			if len(names) > 1 {
				out = append(out, names)
			}
			i += end
		case '(':
			end := strings.IndexByte(literal[i:], ')')
			if end < 0 {
				continue
			}
			inner := literal[i+1 : i+end]
			if !strings.Contains(inner, ",") {
				continue
			}
			names := strings.Split(inner, ",")
			for j := range names {
				names[j] = strings.TrimSpace(names[j])
			}
			if len(names) > 1 {
				out = append(out, names)
			}
			i += end
		}
	}
	return out
}

// sortedNames returns the sorted names of a command.
func sortedNames(names []string) []string {
	out := append([]string{}, names...)
	sort.Strings(out)
	return out
}
