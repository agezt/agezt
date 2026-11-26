// SPDX-License-Identifier: MIT

// Command bodydoor enforces the programme's last rule: every place that
// reads an inbound request body is behind a door, or has declared itself an
// exception.
//
// The daemon's surfaces were given their request vocabulary one slice at a
// time — the `/api/v1` surface, the console, the OpenAI-compatible surface,
// the agent gateway, the fifteen channel webhook members and the ACP server.
// Each of them answers a mistyped body in the operation registry's words:
//
//	operation session_new input: $.cwd has wrong type: got number, want string
//
// But that was a **document**, and a document does not fail a build. Nothing
// stopped a NEW surface from reading a body with no contract, no schema and
// no door — and nothing stopped a surface that HAS a door from growing a
// second, unguarded body.
//
// This command is the ratchet that closes that hole. It walks the tree, finds
// every function that reads a request body, and asks one question:
//
//	is the read behind a door?
//
// A door is a call to the shared core (kernel/httpserver):
//
//	httpserver.DecodeJSONBody(...)  — the REST surface, the console, the
//	                                 OpenAI-compatible surface, the gateway
//	httpserver.CheckJSONBody(...)   — the channel webhook members, the ACP
//	                                 server
//
// A function that reads a body and calls a door directly — or calls a
// same-package function that does — is behind the door. A function that reads
// a body with no door in reach must be DECLARED, in
// tools/bodydoor/exceptions.txt, with the reason it is not measured:
//
//	kernel/webui/transcribe.go :: an audio upload, not a JSON body
//
// and the list is a RATCHET, in the same shape as archcheck's:
//
//   - a body read with no door and no declaration FAILS the check (no new
//     debt);
//   - a declaration whose file no longer reads a body ALSO fails, so a fixed
//     file must be deleted from the list in the same change and can never
//     silently come back.
//
// Usage:
//
//	go run ./tools/bodydoor            # check (CI)
//	go run ./tools/bodydoor -update    # drop declarations that are stale
//	go run ./tools/bodydoor -summary   # counts per package
//
// Exit codes: 0 clean, 1 violations/stale declarations, 2 usage or
// environment error.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// osExit allows tests to intercept os.Exit without terminating.
var osExit = os.Exit

// httpserverPath is the shared door core: a call to one of its door
// functions is what "behind a door" means.
const httpserverPath = "github.com/agezt/agezt/kernel/httpserver"

// doorFuncs are the core's door functions.
var doorFuncs = map[string]bool{
	"DecodeJSONBody": true,
	"CheckJSONBody":  true,
	"RefuseJSONBody": true,
}

// targetOSes are the platforms whose build-constrained files are analysed —
// the union over every shipped platform, so the result does not flip by host
// (the same rule archcheck follows).
var targetOSes = []string{"linux", "windows", "darwin"}

// listPackages is injectable in tests to avoid running `go list`.
var listPackages = func() ([]Pkg, error) {
	var perOS [][]Pkg
	for _, goos := range targetOSes {
		cmd := exec.Command(goBinary(), "list", "-e", "-json=ImportPath,Dir,GoFiles", "./...")
		cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list (GOOS=%s): %v: %s", goos, err, strings.TrimSpace(stderr.String()))
		}
		pkgs, err := decodePackages(bytes.NewReader(out))
		if err != nil {
			return nil, err
		}
		perOS = append(perOS, pkgs)
	}
	return unionPackages(perOS...), nil
}

// Pkg is one package of the tree.
type Pkg struct {
	ImportPath string
	Dir        string
	GoFiles    []string
}

// unionPackages merges per-platform package lists: a package's files are the
// union across platforms, in first-seen order.
func unionPackages(lists ...[]Pkg) []Pkg {
	var out []Pkg
	idx := map[string]int{}
	for _, list := range lists {
		for _, p := range list {
			i, ok := idx[p.ImportPath]
			if !ok {
				i = len(out)
				idx[p.ImportPath] = i
				out = append(out, Pkg{ImportPath: p.ImportPath, Dir: p.Dir})
			}
			out[i].GoFiles = appendUnique(out[i].GoFiles, p.GoFiles...)
		}
	}
	return out
}

func appendUnique(dst []string, add ...string) []string {
	for _, a := range add {
		dup := false
		for _, d := range dst {
			if d == a {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, a)
		}
	}
	return dst
}

func decodePackages(r io.Reader) ([]Pkg, error) {
	dec := json.NewDecoder(r)
	var pkgs []Pkg
	for {
		var p Pkg
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			return pkgs, nil
		} else if err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		pkgs = append(pkgs, p)
	}
}

// goBinary resolves the `go` executable of the toolchain that built this
// binary (same pinning as deadcodecheck and archcheck: a stray `go` shim on
// PATH must not decide what gets analysed).
func goBinary() string {
	//lint:ignore SA1019 bodydoor always runs in-repo via `go run`, never as a
	// copied binary — the building toolchain's GOROOT is exactly the pin we want.
	if root := runtime.GOROOT(); root != "" {
		bin := filepath.Join(root, "bin", "go")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	return "go"
}

// declarationsPath is the file the exceptions are declared in.
var declarationsPath = filepath.Join("tools", "bodydoor", "exceptions.txt")

// main runs the check.
func main() {
	update := flag.Bool("update", false, "drop declarations whose file no longer reads a body")
	summary := flag.Bool("summary", false, "print counts per package")
	flag.Parse()

	pkgs, err := listPackages()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bodydoor:", err)
		osExit(2)
		return
	}
	result, err := analyse(pkgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bodydoor:", err)
		osExit(2)
		return
	}
	if *summary {
		printSummary(result)
		return
	}
	osExit(runChecker(os.Stdout, os.Stderr, result, *update))
}

// runChecker prints the verdict and returns the exit code.
func runChecker(stdout, stderr io.Writer, result Result, update bool) int {
	declared, err := loadDeclarations()
	if err != nil {
		fmt.Fprintln(stderr, "bodydoor:", err)
		return 2
	}
	var violations []Violation
	for _, v := range result.Violations {
		if _, ok := declared[v.File]; ok {
			continue
		}
		violations = append(violations, v)
	}
	var stale []string
	for file := range declared {
		if !result.Files[file] {
			stale = append(stale, file)
		}
	}
	sort.Strings(stale)
	if update && len(stale) > 0 {
		if err := pruneDeclarations(declared, stale); err != nil {
			fmt.Fprintln(stderr, "bodydoor:", err)
			return 2
		}
		fmt.Fprintf(stdout, "bodydoor: dropped %d stale declaration(s)\n", len(stale))
	}
	if len(violations) == 0 && len(stale) == 0 {
		fmt.Fprintf(stdout, "OK: %d request-body read(s), every one behind a door (%d declared exception(s)).\n",
			countReads(result), len(declared))
		return 0
	}
	if len(violations) > 0 {
		fmt.Fprintln(stderr, "bodydoor: a request body is read with no door in reach:")
		for _, v := range violations {
			fmt.Fprintf(stderr, "  %s:%d (%s): %s\n", v.File, v.Line, v.Where, v.Detail)
		}
	}
	if len(stale) > 0 {
		fmt.Fprintln(stderr, "bodydoor: a declared exception reads no body any more (the list may only shrink):")
		for _, file := range stale {
			fmt.Fprintf(stderr, "  %s\n", file)
		}
	}
	return 1
}

func countReads(result Result) int {
	total := 0
	for _, n := range result.Reads {
		total += n
	}
	return total
}

func countBehind(result Result) int {
	total := 0
	for _, n := range result.Behind {
		total += n
	}
	return total
}

func printSummary(result Result) {
	pkgs := make([]string, 0, len(result.Reads))
	for p := range result.Reads {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		fmt.Printf("%-58s reads=%-4d behind-a-door=%d\n", p, result.Reads[p], result.Behind[p])
	}
	fmt.Printf("TOTAL reads=%d behind-a-door=%d\n", countReads(result), countBehind(result))
}

// loadDeclarations reads the declared exceptions: a file and the reason it
// is not measured.
func loadDeclarations() (map[string]string, error) {
	out := map[string]string{}
	file, err := os.Open(declarationsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, reason, _ := strings.Cut(line, "::")
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		out[path] = strings.TrimSpace(reason)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// pruneDeclarations rewrites the declarations file without the stale ones.
func pruneDeclarations(declared map[string]string, stale []string) error {
	drop := map[string]bool{}
	for _, f := range stale {
		drop[f] = true
	}
	var kept []string
	for file, reason := range declared {
		if drop[file] {
			continue
		}
		kept = append(kept, fmt.Sprintf("%s :: %s", file, reason))
	}
	sort.Strings(kept)
	body := "# The request bodies this tree reads WITHOUT a door, and why.\n" +
		"#\n" +
		"# A body read with no door in reach must be declared here — the list is a\n" +
		"# RATCHET: a declaration whose file no longer reads a body fails the check,\n" +
		"# so a fixed file must be deleted in the same change.\n"
	for _, line := range kept {
		body += line + "\n"
	}
	return os.WriteFile(declarationsPath, []byte(body), 0o644)
}
