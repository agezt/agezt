// SPDX-License-Identifier: MIT

package main

// Coverage tests for the sdkparity command. The pre-existing main_test.go covers
// routesForSDKCoverage / intentionallyUnsupportedInSDK; this file exercises the
// remaining functions — extractRoutes, mustGlob, render, sdkCovers, noteForRoute,
// normalize, and main's -out / -check happy paths.
//
// fatal() calls os.Exit(1); Go's -coverprofile atexit writer is bypassed by
// os.Exit, so fatal and the fatal-terminated error branches of main cannot be
// measured without a source refactor that injects the exit behavior. They are
// the only statements that stay uncovered and are documented here.

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractRoutesSuccess covers extractRoutes' happy path: it reads a Go
// source file, pulls every distinct /api/v1/ mux.HandleFunc registration, marks
// trailing-slash routes as dynamic, sorts them, and dedupes repeats.
func TestExtractRoutesSuccess(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "restapi.go")
	// Includes: two /api/v1 routes (one dynamic), a duplicate to exercise the
	// seen[] dedupe, and a non-/api/v1 route that must be skipped.
	content := `package restapi
func register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/runs/", runs)
	mux.HandleFunc("/api/v1/health", health)
	mux.HandleFunc("/api/v1/health", health) // duplicate, must dedupe
	mux.HandleFunc("/internal/debug", debug) // non-v1, must skip
}
`
	if err := os.WriteFile(src, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp source: %v", err)
	}

	routes, err := extractRoutes(src)
	if err != nil {
		t.Fatalf("extractRoutes error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("got %d routes, want 2: %#v", len(routes), routes)
	}
	// sort.Slice orders "/api/v1/health" before "/api/v1/runs/".
	if routes[0].Path != "/api/v1/health" || routes[0].Dynamic {
		t.Errorf("routes[0] = %#v, want exact /api/v1/health", routes[0])
	}
	if routes[1].Path != "/api/v1/runs/" || !routes[1].Dynamic {
		t.Errorf("routes[1] = %#v, want dynamic /api/v1/runs/", routes[1])
	}
}

// TestExtractRoutesMissingFile covers extractRoutes' read-error branch.
func TestExtractRoutesMissingFile(t *testing.T) {
	if _, err := extractRoutes(filepath.Join(t.TempDir(), "nope.go")); err == nil {
		t.Fatal("expected error reading a missing file")
	}
}

// TestMustGlobSuccess covers mustGlob's happy path (a valid pattern returns the
// sorted matches). The error branch calls fatal→os.Exit and is not unit-testable.
func TestMustGlobSuccess(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.go", "a.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	got := mustGlob(filepath.Join(dir, "*.go"))
	if len(got) != 2 {
		t.Fatalf("mustGlob matched %d files, want 2: %v", len(got), got)
	}
	// sort.Strings makes the result deterministic.
	if !strings.HasSuffix(got[0], "a.go") || !strings.HasSuffix(got[1], "b.go") {
		t.Errorf("mustGlob results not sorted: %v", got)
	}
}

// TestMustGlobNoMatch covers the case where the pattern is valid but matches
// nothing — filepath.Glob returns (nil, nil), so mustGlob returns an empty slice.
func TestMustGlobNoMatch(t *testing.T) {
	got := mustGlob(filepath.Join(t.TempDir(), "*.does-not-exist"))
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

// TestSdkCovers exercises every branch of sdkCovers: an exact-path hit, a
// dynamic-route hit via the trailing-slash-trimmed form, a miss, and the
// read-error path.
func TestSdkCovers(t *testing.T) {
	dir := t.TempDir()
	exactFile := filepath.Join(dir, "exact.ts")
	if err := os.WriteFile(exactFile, []byte(`fetch("/api/v1/health")`), 0o600); err != nil {
		t.Fatalf("write exact: %v", err)
	}
	dynFile := filepath.Join(dir, "dyn.ts")
	// Contains the trimmed form "/api/v1/runs" (no trailing slash) so the
	// Dynamic branch of sdkCovers matches.
	if err := os.WriteFile(dynFile, []byte(`fetch("/api/v1/runs/" + id)`), 0o600); err != nil {
		t.Fatalf("write dyn: %v", err)
	}
	trimFile := filepath.Join(dir, "trim.ts")
	if err := os.WriteFile(trimFile, []byte(`const base = "/api/v1/runs"`), 0o600); err != nil {
		t.Fatalf("write trim: %v", err)
	}

	// Exact hit.
	ok, err := sdkCovers([]string{exactFile}, route{Path: "/api/v1/health"})
	if err != nil || !ok {
		t.Errorf("exact match: ok=%v err=%v, want ok=true", ok, err)
	}

	// Dynamic hit via the trimmed-suffix branch (file has the exact path too,
	// but this asserts trimmed matching works when only the base is present).
	ok, err = sdkCovers([]string{trimFile}, route{Path: "/api/v1/runs/", Dynamic: true})
	if err != nil || !ok {
		t.Errorf("dynamic trimmed match: ok=%v err=%v, want ok=true", ok, err)
	}

	// Miss: needle not present in the file.
	ok, err = sdkCovers([]string{exactFile}, route{Path: "/api/v1/models"})
	if err != nil || ok {
		t.Errorf("miss: ok=%v err=%v, want ok=false", ok, err)
	}

	// Read error: a path that does not exist.
	if _, err := sdkCovers([]string{filepath.Join(dir, "gone.ts")}, route{Path: "/x"}); err == nil {
		t.Error("expected read error for missing SDK file")
	}
}

// TestNoteForRoute covers every case of noteForRoute's switch, including the
// prefix matches and the default empty note.
func TestNoteForRoute(t *testing.T) {
	cases := map[string]string{
		"/api/v1/health":         "daemon health",
		"/api/v1/models":         "model catalog",
		"/api/v1/runs":           "blocking + streaming run creation",
		"/api/v1/runs/":          "run event arc by correlation id",
		"/api/v1/mailbox":        "inter-agent mailbox",
		"/api/v1/mailbox/send":   "inter-agent mailbox",
		"/api/v1/update":         "admin self-update endpoint",
		"/api/v1/update/apply":   "admin self-update endpoint",
		"/api/v1/something/else": "",
	}
	for path, want := range cases {
		if got := noteForRoute(path); got != want {
			t.Errorf("noteForRoute(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestNormalize covers normalize: CRLF is folded to LF and the result is
// trimmed then given exactly one trailing newline.
func TestNormalize(t *testing.T) {
	got := normalize([]byte("\r\n  line one\r\nline two  \r\n\r\n"))
	want := "line one\nline two\n"
	if string(got) != want {
		t.Errorf("normalize = %q, want %q", got, want)
	}
}

// TestRender covers render end to end, including all cell branches: an exact and
// a dynamic route, a NativeOnly SDK (n/a column), a covering SDK (✅), a
// non-covering SDK (—), a route with a note, and the summary lines.
func TestRender(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "client.py")
	// Python covers /api/v1/health but not /api/v1/runs/.
	if err := os.WriteFile(pyFile, []byte(`url = "/api/v1/health"`), 0o600); err != nil {
		t.Fatalf("write py: %v", err)
	}

	routes := []route{
		{Path: "/api/v1/health"},
		{Path: "/api/v1/runs/", Dynamic: true},
	}
	sdks := []sdk{
		{Name: "Go", NativeOnly: true},
		{Name: "Python", Files: []string{pyFile}},
	}

	out, err := render(routes, sdks)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	s := string(out)

	for _, want := range []string{
		"# SDK Parity Report",
		"| `/api/v1/health` | exact | n/a | ✅", // exact + NativeOnly n/a + covered
		"| `/api/v1/runs/` | prefix | n/a | —", // dynamic + not covered
		"daemon health",                        // note column
		"**Go**: n/a for REST route-string coverage",
		"**Python**: 1/2 SDK-intended REST routes", // summary count
		"## Intentionally unsupported SDK routes",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("render output missing %q\n---\n%s", want, s)
		}
	}
}

// TestRenderPropagatesReadError covers render's error propagation: when
// sdkCovers hits an unreadable file, render returns that error.
func TestRenderPropagatesReadError(t *testing.T) {
	sdks := []sdk{{Name: "Python", Files: []string{filepath.Join(t.TempDir(), "missing.py")}}}
	if _, err := render([]route{{Path: "/api/v1/health"}}, sdks); err == nil {
		t.Fatal("expected render to propagate the sdkCovers read error")
	}
}

// TestMainOutPath drives main() in-process with -out, exercising the flag parse,
// route extraction from the real kernel/restapi/restapi.go, render, and the
// file-write return path. Running from the repo root lets extractRoutes and the
// mustGlob SDK lookups resolve their real relative paths.
func TestMainOutPath(t *testing.T) {
	restore := chdirTo(t, repoRoot(t))
	defer restore()

	out := filepath.Join(t.TempDir(), "report.md")
	withArgs(t, []string{"sdkparity", "-out", out}, func() {
		main()
	})

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("main -out did not write report: %v", err)
	}
	if !bytes.Contains(data, []byte("# SDK Parity Report")) {
		t.Errorf("report missing header:\n%s", data)
	}
}

// TestMainCheckPathMatches drives main() with -check against a file that matches
// the freshly generated report, exercising the ReadFile + bytes.Equal success
// branch (which returns without os.Exit).
func TestMainCheckPathMatches(t *testing.T) {
	restore := chdirTo(t, repoRoot(t))
	defer restore()

	// First generate the canonical report to a temp file...
	golden := filepath.Join(t.TempDir(), "golden.md")
	withArgs(t, []string{"sdkparity", "-out", golden}, func() { main() })

	// ...then run -check against it; identical content takes the success path.
	withArgs(t, []string{"sdkparity", "-check", golden}, func() { main() })
}

// withArgs runs fn with os.Args and the flag package's command line reset to the
// supplied args, restoring both afterward. main() calls flag.Parse on the
// default CommandLine, so it must be reset between invocations.
func withArgs(t *testing.T, args []string, fn func()) {
	t.Helper()
	origArgs := os.Args
	origFlags := flag.CommandLine
	defer func() {
		os.Args = origArgs
		flag.CommandLine = origFlags
	}()
	os.Args = args
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ExitOnError)
	fn()
}

// repoRoot walks up from the working directory to the module root (the dir with
// go.mod) so the tests find the real kernel/restapi/restapi.go and sdk/ trees.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", dir)
		}
		dir = parent
	}
}

// chdirTo switches the working directory and returns a restore func.
func chdirTo(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restore chdir %s: %v", orig, err)
		}
	}
}
