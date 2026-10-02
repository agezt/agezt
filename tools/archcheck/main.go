// SPDX-License-Identifier: MIT

// Command archcheck enforces the layer architecture described in
// architecture/20-target-architecture.md: every package is placed in a layer
// (and, for L3, a module) by tools/archcheck/layers.json, and every in-module
// import edge must respect four rules — no upward dependency, no module
// reaching into another module's internals, no adapter bypassing the app
// layer, no plugin reaching past contracts and platform (see rules.go).
//
// It also enforces "one way to do each infrastructural thing" (calls.go):
// starting a child process, building an HTTP client and writing a file raw
// are only legitimate in the platform package that owns the guarantee
// (layers.json "calls"); everywhere else they are counted per package.
//
// The existing tree violates these rules in many places; that is the work
// of architecture/21-migration-roadmap.md. The violations are recorded in
// tools/archcheck/allowlist.txt (import edges) and
// tools/archcheck/calls-allowlist.txt (call-site counts per package); both
// are RATCHETS:
//
//   - a violation that is not allowlisted fails the check (no new debt);
//   - an allowlist entry that no longer occurs ALSO fails the check, so a
//     fixed edge must be deleted from the allowlist in the same change and
//     can never silently come back;
//   - a package that no rule classifies fails the check, so a new package
//     must be placed in the architecture before it can merge.
//
// Usage:
//
//	go run ./tools/archcheck            # check (CI)
//	go run ./tools/archcheck -update    # drop allowlist entries that were fixed
//	go run ./tools/archcheck -update -allow-new   # also accept NEW violations (reviewer-visible)
//	go run ./tools/archcheck -summary   # counts per kind and per importer
//
// Exit codes: 0 clean, 1 violations/stale entries/unmapped packages, 2 usage
// or environment error.
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
	"strconv"
	"strings"
)

// osExit allows tests to intercept os.Exit without terminating.
var osExit = os.Exit

// goBinary resolves the `go` executable of the toolchain this binary was
// built with, falling back to PATH lookup (same pinning as deadcodecheck:
// a stray `go` shim on PATH must not decide what gets analysed).
func goBinary() string {
	//lint:ignore SA1019 archcheck always runs in-repo via `go run`, never as
	// a copied binary — the building toolchain's GOROOT is exactly the pin we want.
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

// listPackages is injectable in tests to avoid running `go list`.
var listPackages = func() ([]Pkg, error) {
	cmd := exec.Command(goBinary(), "list", "-e", "-json=ImportPath,Imports,Dir,GoFiles", "./...")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return decodePackages(bytes.NewReader(out))
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

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("archcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "tools/archcheck/layers.json", "layer map")
	allowPath := fs.String("allowlist", "tools/archcheck/allowlist.txt", "import-edge ratchet allowlist")
	callsPath := fs.String("calls-allowlist", "tools/archcheck/calls-allowlist.txt", "forbidden-call ratchet allowlist")
	update := fs.Bool("update", false, "rewrite the allowlists, dropping fixed entries")
	allowNew := fs.Bool("allow-new", false, "with -update: also accept new violations")
	summary := fs.Bool("summary", false, "print violation counts per kind and per importer")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *allowNew && !*update {
		fmt.Fprintln(stderr, "archcheck: -allow-new requires -update")
		return 2
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "archcheck: %v\n", err)
		return 2
	}
	pkgs, err := listPackages()
	if err != nil {
		fmt.Fprintf(stderr, "archcheck: %v\n", err)
		return 2
	}
	rep := evaluate(cfg, pkgs)
	calls, err := scanCalls(cfg, pkgs)
	if err != nil {
		fmt.Fprintf(stderr, "archcheck: %v\n", err)
		return 2
	}

	allowed, err := readAllowlist(*allowPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stderr, "archcheck: read allowlist: %v\n", err)
		return 2
	}
	allowedCalls, err := readCallsAllowlist(*callsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stderr, "archcheck: read calls allowlist: %v\n", err)
		return 2
	}
	diff := compare(rep, allowed)
	cdiff := compareCalls(calls, allowedCalls)

	if *summary {
		printSummary(stdout, rep, calls)
	}

	if len(rep.Unmapped) > 0 {
		fmt.Fprintln(stderr, "ERROR: packages not placed in the architecture (add a rule to tools/archcheck/layers.json):")
		for _, p := range rep.Unmapped {
			fmt.Fprintln(stderr, "  -", p)
		}
		return 1
	}

	if *update {
		keep, keepCalls := diff.stillAllowed, cdiff.stillAllowed
		if *allowNew {
			keep, keepCalls = diff.current, cdiff.current
		}
		if err := writeAllowlist(*allowPath, keep); err != nil {
			fmt.Fprintf(stderr, "archcheck: write allowlist: %v\n", err)
			return 2
		}
		if err := writeCallsAllowlist(*callsPath, keepCalls); err != nil {
			fmt.Fprintf(stderr, "archcheck: write calls allowlist: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "archcheck: allowlists rewritten: %d import edges, %d call entries (%d+%d fixed entries tightened",
			len(keep), len(keepCalls), len(diff.stale), len(cdiff.stale))
		if *allowNew {
			fmt.Fprintf(stdout, ", %d+%d new violations accepted)\n", len(diff.added), len(cdiff.added))
			return 0
		}
		fmt.Fprintln(stdout, ")")
		if len(diff.added)+len(cdiff.added) > 0 {
			printAdded(stderr, diff.added, cdiff.added)
			return 1
		}
		return 0
	}

	failed := false
	if len(diff.added)+len(cdiff.added) > 0 {
		printAdded(stderr, diff.added, cdiff.added)
		failed = true
	}
	if stale := append(append([]string(nil), diff.stale...), cdiff.stale...); len(stale) > 0 {
		fmt.Fprintln(stderr, "ERROR: allowlisted violations that no longer occur (or occur less) — the debt was paid;")
		fmt.Fprintln(stderr, "tighten the allowlist so it cannot come back (go run ./tools/archcheck -update):")
		for _, k := range stale {
			fmt.Fprintln(stderr, "  -", k)
		}
		failed = true
	}
	if failed {
		return 1
	}
	fmt.Fprintf(stdout, "OK: %d packages placed; %d allowlisted import violations and %d allowlisted call sites remain (ratchet).\n",
		rep.Packages, len(diff.stillAllowed), sumCounts(cdiff.stillAllowed))
	return 0
}

func printAdded(w io.Writer, edges, calls []string) {
	if len(edges) > 0 {
		fmt.Fprintln(w, "ERROR: new architecture violations (see architecture/20-target-architecture.md §1-§2):")
		for _, k := range edges {
			fmt.Fprintln(w, "  -", k)
		}
		fmt.Fprintln(w, "Fix the dependency (move the type to a contract package, call the module's api,")
		fmt.Fprintln(w, "go through an app operation). Allowlisting is for pre-existing debt only.")
	}
	if len(calls) > 0 {
		fmt.Fprintln(w, "ERROR: new forbidden call sites (architecture/20-target-architecture.md P3/P7):")
		for _, k := range calls {
			fmt.Fprintln(w, "  -", k)
		}
		fmt.Fprintln(w, "Start children through the sandbox launcher, dial out through the guarded client,")
		fmt.Fprintln(w, "write files through internal/atomicfile or the store framework.")
	}
}

type allowDiff struct {
	current      []string // every violation key found now
	stillAllowed []string // found now AND allowlisted
	added        []string // found now, NOT allowlisted
	stale        []string // allowlisted, NOT found now
}

func compare(rep Report, allowed map[string]bool) allowDiff {
	var d allowDiff
	found := map[string]bool{}
	for _, v := range rep.Violations {
		k := v.Key()
		found[k] = true
		d.current = append(d.current, k)
		if allowed[k] {
			d.stillAllowed = append(d.stillAllowed, k)
		} else {
			d.added = append(d.added, k)
		}
	}
	for k := range allowed {
		if !found[k] {
			d.stale = append(d.stale, k)
		}
	}
	sort.Strings(d.stale)
	return d
}

func readAllowlist(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return map[string]bool{}, err
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, sc.Err()
}

const allowlistHeader = `# archcheck ratchet allowlist — pre-existing architecture debt.
# Generated by: go run ./tools/archcheck -update
# Format: <kind> <importer> -> <imported>   (module-relative package paths)
# Every entry is an edge architecture/21-migration-roadmap.md removes. Entries
# may only be DELETED (the check fails when a listed edge no longer occurs).
`

func writeAllowlist(path string, keys []string) error {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	var b strings.Builder
	b.WriteString(allowlistHeader)
	for _, k := range sorted {
		b.WriteString(k)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func printSummary(w io.Writer, rep Report, calls []CallCount) {
	byKind := map[string]int{}
	byImporter := map[string]int{}
	for _, v := range rep.Violations {
		byKind[v.Kind]++
		byImporter[v.Importer]++
	}
	fmt.Fprintf(w, "packages: %d  violations: %d\n", rep.Packages, len(rep.Violations))
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(w, "  %-15s %d\n", k, byKind[k])
	}
	type kv struct {
		k string
		n int
	}
	var imps []kv
	for k, n := range byImporter {
		imps = append(imps, kv{k, n})
	}
	sort.Slice(imps, func(i, j int) bool {
		if imps[i].n != imps[j].n {
			return imps[i].n > imps[j].n
		}
		return imps[i].k < imps[j].k
	})
	byRule := map[string]int{}
	for _, c := range calls {
		byRule[c.Rule] += c.N
	}
	for _, r := range []string{CallExec, CallHTTPClient, CallRawWrite} {
		fmt.Fprintf(w, "  call:%-10s %d sites\n", r, byRule[r])
	}
	fmt.Fprintln(w, "top importers:")
	for i, e := range imps {
		if i == 15 {
			break
		}
		fmt.Fprintf(w, "  %4d  %s\n", e.n, e.k)
	}
}

func sumCounts(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func readCallsAllowlist(path string) (map[string]int, error) {
	lines, err := readAllowlist(path)
	out := map[string]int{}
	for line := range lines {
		f := strings.Fields(line)
		if len(f) != 3 {
			return out, fmt.Errorf("%s: malformed line %q (want: <rule> <package> <count>)", path, line)
		}
		n, convErr := strconv.Atoi(f[2])
		if convErr != nil || n <= 0 {
			return out, fmt.Errorf("%s: bad count in %q", path, line)
		}
		out[f[0]+" "+f[1]] = n
	}
	return out, err
}

const callsAllowlistHeader = `# archcheck forbidden-call ratchet — pre-existing call sites that bypass a
# platform guarantee (exec: sandbox launcher · http-client: guarded client ·
# raw-write: atomic writer / store framework).
# Generated by: go run ./tools/archcheck -update
# Format: <rule> <package> <count>. Counts may only go DOWN; a package whose
# count reaches 0 is removed. Where each rule is legitimate: layers.json "calls".
`

func writeCallsAllowlist(path string, entries map[string]int) error {
	keys := make([]string, 0, len(entries))
	for k, n := range entries {
		if n > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(callsAllowlistHeader)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %d\n", k, entries[k])
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
