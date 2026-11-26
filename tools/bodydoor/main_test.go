// SPDX-License-Identifier: MIT

package main

// The tool's own guards.
//
// The tool exists to catch a request body that is read with no door in reach
// — so its own question is the one thing it must answer right. The tests
// below hold it to that with a fixture tree: a body behind a door passes, a
// body with no door is reported, a response body is not a request body, and a
// declaration that has gone stale fails (the ratchet).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture writes a file into a tree and returns the tree's root.
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// analyseTree runs the analysis over a fixture tree.
func analyseTree(t *testing.T, root string, files ...string) Result {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("chdir back: %v", err)
		}
	}()
	result, err := analyse([]Pkg{{ImportPath: "example/pkg", Dir: root, GoFiles: files}})
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	return result
}

// TestAReadBehindADoorIsNotAViolation: the tool's question is "is the read
// behind a door?", and a read that calls the shared core IS behind one.
func TestAReadBehindADoorIsNotAViolation(t *testing.T) {
	root := fixture(t, map[string]string{
		"handler.go": `package pkg

import (
	"io"
	"net/http"

	"github.com/agezt/agezt/kernel/httpserver"
)

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var in struct{}
	if err := httpserver.CheckJSONBody(body, nil, "op", &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
`,
	})
	result := analyseTree(t, root, "handler.go")
	if len(result.Violations) != 0 {
		t.Fatalf("a read behind a door was reported: %+v", result.Violations)
	}
	if result.Reads["example/pkg"] != 1 {
		t.Errorf("reads = %d, want 1", result.Reads["example/pkg"])
	}
	if result.Behind["example/pkg"] != 1 {
		t.Errorf("behind a door = %d, want 1", result.Behind["example/pkg"])
	}
}

// TestAReadBehindALocalDoorIsNotAViolation: the door may live one call away,
// in a helper of the same package — that is how the console and the gateway
// do it.
func TestAReadBehindALocalDoorIsNotAViolation(t *testing.T) {
	root := fixture(t, map[string]string{
		"handler.go": `package pkg

import (
	"io"
	"net/http"
)

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	decode(w, body)
}
`,
		"door.go": `package pkg

import (
	"net/http"

	"github.com/agezt/agezt/kernel/httpserver"
)

func decode(w http.ResponseWriter, body []byte) {
	var in struct{}
	if err := httpserver.DecodeJSONBody(nil, nil, "op", &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
`,
	})
	result := analyseTree(t, root, "handler.go", "door.go")
	if len(result.Violations) != 0 {
		t.Fatalf("a read behind a local door was reported: %+v", result.Violations)
	}
	if result.Behind["example/pkg"] != 1 {
		t.Errorf("behind a door = %d, want 1", result.Behind["example/pkg"])
	}
}

// TestAReadWithNoDoorIsAViolation: the hole the tool exists to catch.
func TestAReadWithNoDoorIsAViolation(t *testing.T) {
	root := fixture(t, map[string]string{
		"handler.go": `package pkg

import (
	"io"
	"net/http"
)

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = body
}
`,
	})
	result := analyseTree(t, root, "handler.go")
	if len(result.Violations) != 1 {
		t.Fatalf("violations = %d, want 1: %+v", len(result.Violations), result.Violations)
	}
	v := result.Violations[0]
	if !strings.HasSuffix(v.File, "handler.go") {
		t.Errorf("file = %q, want handler.go", v.File)
	}
	if v.Where != "handle" {
		t.Errorf("where = %q, want handle", v.Where)
	}
	if result.Behind["example/pkg"] != 0 {
		t.Errorf("behind a door = %d, want 0", result.Behind["example/pkg"])
	}
}

// TestAResponseBodyIsNotARequestBody: an outbound client reads the RESPONSE
// body, and that is none of this tool's business.
func TestAResponseBodyIsNotARequestBody(t *testing.T) {
	root := fixture(t, map[string]string{
		"client.go": `package pkg

import (
	"io"
	"net/http"
)

func fetch(resp *http.Response) {
	body, _ := io.ReadAll(resp.Body)
	_ = body
}
`,
	})
	result := analyseTree(t, root, "client.go")
	if len(result.Violations) != 0 {
		t.Fatalf("a response body was reported: %+v", result.Violations)
	}
	if result.Reads["example/pkg"] != 0 {
		t.Errorf("reads = %d, want 0", result.Reads["example/pkg"])
	}
}

// TestAFunctionOfTheSameNameIsNotADoor: only the SHARED CORE's door
// functions count. A local function that happens to be called CheckJSONBody
// is not the door.
func TestAFunctionOfTheSameNameIsNotADoor(t *testing.T) {
	root := fixture(t, map[string]string{
		"handler.go": `package pkg

import (
	"io"
	"net/http"
)

type fake struct{}

func (fake) CheckJSONBody(body []byte) {}

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	fake{}.CheckJSONBody(body)
}
`,
	})
	result := analyseTree(t, root, "handler.go")
	if len(result.Violations) != 1 {
		t.Fatalf("violations = %d, want 1: %+v", len(result.Violations), result.Violations)
	}
}

// TestTheCheckerExemptsADeclaredFileAndFailsAStaleOne: the declarations are
// the exceptions, and they are a RATCHET — a file that no longer reads a body
// must fail the check so the list can only shrink.
func TestTheCheckerExemptsADeclaredFileAndFailsAStaleOne(t *testing.T) {
	declared := filepath.Join(t.TempDir(), "exceptions.txt")
	previous := declarationsPath
	declarationsPath = declared
	t.Cleanup(func() { declarationsPath = previous })

	result := Result{
		Violations: []Violation{{
			Finding: Finding{File: "pkg/reader.go", Line: 10, Where: "handle", Detail: "reads the request body"},
			Package: "example/pkg",
		}},
		Behind: map[string]int{},
		Reads:  map[string]int{"example/pkg": 1},
		Files:  map[string]bool{"pkg/reader.go": true},
	}

	// No declaration yet: the violation is reported.
	if code := runChecker(os.Stdout, os.Stderr, result, false); code != 1 {
		t.Fatalf("an undeclared read passed the check (code %d)", code)
	}

	// The file is declared: the read is excused.
	if err := os.WriteFile(declared, []byte("pkg/reader.go :: an upload, not a JSON body\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if code := runChecker(os.Stdout, os.Stderr, result, false); code != 0 {
		t.Fatalf("a declared read failed the check (code %d)", code)
	}

	// The file stops reading a body: the declaration is stale, and the
	// ratchet says so.
	result.Files = map[string]bool{}
	if code := runChecker(os.Stdout, os.Stderr, result, false); code != 1 {
		t.Fatalf("a stale declaration passed the check (code %d)", code)
	}
	// -update drops it.
	if code := runChecker(os.Stdout, os.Stderr, result, true); code != 1 {
		t.Fatalf("the stale declaration was not reported (code %d)", code)
	}
	body, err := os.ReadFile(declared)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(body), "pkg/reader.go") {
		t.Errorf("the stale declaration survived -update:\n%s", body)
	}
}
