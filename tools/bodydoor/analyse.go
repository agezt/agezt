// SPDX-License-Identifier: MIT

package main

// The analysis: where the tree reads a request body, and whether a door is
// in reach.
//
// The question is asked per FUNCTION, not per package: a package that has a
// door may still grow a second handler that reads a body with no door, and
// that is exactly the hole this ratchet closes.
//
// Two things are found:
//
//   - a body READ: a use of `<request>.Body`, where `<request>` is a
//     parameter of type *http.Request (or http.Request) of the function being
//     analysed. The type is what decides — a response body (`resp.Body`)
//     is an outbound read and is none of this tool's business.
//   - a DOOR: a call to one of the shared core's door functions. A function
//     that calls a door directly is behind it; so is a function that calls a
//     same-package function that does, and so on — the closure is computed
//     per package, to a fixed point.
//
// The names are matched by name within a package, not by type: a package
// with two methods called `decode` would let one stand for the other. The
// door functions of this tree have distinctive names, and a false "behind a
// door" would need two same-named methods in one package, one of which is a
// door — which the tree does not have.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding is one place that reads a request body.
type Finding struct {
	// File is the repository-relative path of the reading file.
	File string
	// Line is where the body is read.
	Line int
	// Where names the function that reads it.
	Where string
	// Detail says what was read.
	Detail string
}

// Violation is a finding with the surface it belongs to.
type Violation struct {
	Finding
	// Package is the import path of the reading package.
	Package string
}

// Result is the whole analysis of the tree.
type Result struct {
	// Violations are the body reads with no door in reach.
	Violations []Violation
	// Behind counts the reading functions that ARE behind a door, per package.
	Behind map[string]int
	// Reads counts every body read the tree makes, per package.
	Reads map[string]int
	// Files is every file that reads a body, whether behind a door or not.
	Files map[string]bool
}

// unit is one analysed function.
type unit struct {
	// pkg is the import path of the package the function lives in.
	pkg string
	// file is the repository-relative path of the file.
	file string
	// name is the display name of the function.
	name string
	// line is where the function is declared.
	line int
	// reqParams are the names of the function's *http.Request parameters.
	reqParams map[string]bool
	// reads are the body reads the function makes.
	reads []Finding
	// calls are the same-package functions the function calls.
	calls map[string]bool
	// directDoor says the function calls a door of the shared core itself.
	directDoor bool
}

// analyse walks the packages and answers the question.
func analyse(pkgs []Pkg) (Result, error) {
	root, err := os.Getwd()
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Behind: map[string]int{},
		Reads:  map[string]int{},
		Files:  map[string]bool{},
	}
	for _, pkg := range pkgs {
		units, err := analysePackage(pkg, root)
		if err != nil {
			return Result{}, err
		}
		behind := doorClosure(units)
		for _, u := range units {
			if len(u.reads) == 0 {
				continue
			}
			result.Reads[u.pkg]++
			result.Files[u.file] = true
			if behind[u.name] {
				result.Behind[u.pkg]++
				continue
			}
			for _, read := range u.reads {
				result.Violations = append(result.Violations, Violation{
					Finding: Finding{
						File:   u.file,
						Line:   read.Line,
						Where:  u.name,
						Detail: read.Detail,
					},
					Package: u.pkg,
				})
			}
		}
	}
	sort.Slice(result.Violations, func(i, j int) bool {
		if result.Violations[i].File != result.Violations[j].File {
			return result.Violations[i].File < result.Violations[j].File
		}
		return result.Violations[i].Line < result.Violations[j].Line
	})
	return result, nil
}

// analysePackage reads one package's files and returns its functions.
func analysePackage(pkg Pkg, root string) ([]*unit, error) {
	fset := token.NewFileSet()
	var units []*unit
	for _, name := range pkg.GoFiles {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(pkg.Dir, name)
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		imports := importNames(file)
		units = append(units, collectUnits(fset, file, pkg.ImportPath, rel, imports)...)
	}
	return units, nil
}

// importNames maps the local name of every import to its path.
func importNames(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		out[name] = path
	}
	return out
}

// collectUnits returns the functions of one file: the top-level declarations
// and the function literals, each as its own unit.
func collectUnits(fset *token.FileSet, file *ast.File, pkg, rel string, imports map[string]string) []*unit {
	var units []*unit
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		units = append(units, collectUnit(fset, fn, pkg, rel, imports, fn.Name.Name))
	}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		units = append(units, collectUnit(fset, lit, pkg, rel, imports, "func literal"))
		return true
	})
	return units
}

// collectUnit analyses one function (a declaration or a literal).
func collectUnit(fset *token.FileSet, fn ast.Node, pkg, rel string, imports map[string]string, name string) *unit {
	u := &unit{
		pkg:       pkg,
		file:      rel,
		name:      name,
		reqParams: map[string]bool{},
		calls:     map[string]bool{},
	}
	var fields *ast.FieldList
	var body *ast.BlockStmt
	switch f := fn.(type) {
	case *ast.FuncDecl:
		fields = f.Type.Params
		body = f.Body
		u.name = f.Name.Name
		u.line = fset.Position(f.Pos()).Line
	case *ast.FuncLit:
		fields = f.Type.Params
		body = f.Body
		u.line = fset.Position(f.Pos()).Line
		u.name = fmt.Sprintf("%s (line %d)", name, u.line)
	}
	for _, field := range requestParams(fields) {
		u.reqParams[field] = true
	}
	if body == nil {
		return u
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if node.Sel.Name != "Body" {
				return true
			}
			ident, ok := node.X.(*ast.Ident)
			if !ok || !u.reqParams[ident.Name] {
				return true
			}
			u.reads = append(u.reads, Finding{
				File:   rel,
				Line:   fset.Position(node.Pos()).Line,
				Where:  u.name,
				Detail: "reads the request body",
			})
		case *ast.CallExpr:
			switch callee := node.Fun.(type) {
			case *ast.SelectorExpr:
				ident, ok := callee.X.(*ast.Ident)
				if !ok {
					return true
				}
				if imports[ident.Name] == httpserverPath && doorFuncs[callee.Sel.Name] {
					u.directDoor = true
					return true
				}
				u.calls[callee.Sel.Name] = true
			case *ast.Ident:
				u.calls[callee.Name] = true
			}
		}
		return true
	})
	return u
}

// requestParams lists the names of the parameters that are an *http.Request.
func requestParams(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var out []string
	for _, field := range fields.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Request" {
			continue
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "http" {
			continue
		}
		for _, name := range field.Names {
			out = append(out, name.Name)
		}
	}
	return out
}

// doorClosure answers which of a package's functions are behind a door: the
// ones that call a door themselves, plus the ones that call such a function,
// to a fixed point.
func doorClosure(units []*unit) map[string]bool {
	behind := map[string]bool{}
	byName := map[string]*unit{}
	for _, u := range units {
		if u.directDoor {
			behind[u.name] = true
		}
		byName[u.name] = u
	}
	changed := true
	for changed {
		changed = false
		for _, u := range units {
			if behind[u.name] {
				continue
			}
			for called := range u.calls {
				if behind[called] {
					behind[u.name] = true
					changed = true
					break
				}
			}
		}
	}
	return behind
}
