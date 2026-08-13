// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withFileRoot points AGEZT_FILE_ROOT at a temp dir for the duration of one
// test, and resets it back to its prior value on cleanup. The route reads
// the env at every request, so this scoping is sufficient.
func withFileRoot(t *testing.T, dir string) {
	t.Helper()
	prior, had := os.LookupEnv("AGEZT_FILE_ROOT")
	if err := os.Setenv("AGEZT_FILE_ROOT", dir); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("AGEZT_FILE_ROOT", prior)
		} else {
			_ = os.Unsetenv("AGEZT_FILE_ROOT")
		}
	})
}

func httpJSON(t *testing.T, h http.Handler, method, target string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFiles_Routes_RequireAuth(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	for _, path := range []string{
		"/api/files/tree?path=",
		"/api/files/raw?path=",
		"/api/files/mkdir",
		"/api/files/rename",
		"/api/files/delete",
	} {
		rec := httpJSON(t, s.Handler(), http.MethodGet, path, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without token: code=%d, want 401", path, rec.Code)
		}
	}
}

func TestFiles_RootDir_CreatedOnFirstUse(t *testing.T) {
	root := t.TempDir()
	withFileRoot(t, root)
	s, _ := newServer(t, &fakeCaller{}, "secret")

	rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/files/tree?path=&token=secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tree empty root: code=%d body=%s", rec.Code, rec.Body.String())
	}
	fi, err := os.Stat(root)
	if err != nil {
		t.Fatalf("root not created: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("root is not a directory: %v", fi)
	}
}

func TestFiles_Tree_ReturnsNodesInCanonicalOrder(t *testing.T) {
	root := t.TempDir()
	withFileRoot(t, root)
	s, _ := newServer(t, &fakeCaller{}, "secret")
	for _, name := range []string{"README.md", "notes/zebra", "notes/alpha", "scratch.txt"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o700); err != nil {
			t.Fatalf("setup mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("hello"), 0o600); err != nil {
			t.Fatalf("setup write %s: %v", name, err)
		}
	}

	rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/files/tree?path=notes&token=secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tree notes: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp fileTreeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if resp.Root != "notes" {
		t.Fatalf("root=%q, want notes", resp.Root)
	}
	got := make([]string, 0, len(resp.Nodes))
	for _, n := range resp.Nodes {
		if n.Type != "dir" && n.Type != "file" {
			t.Fatalf("node %s: bad type %q", n.Name, n.Type)
		}
		got = append(got, n.Name)
	}
	want := []string{"alpha", "zebra"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order=%v, want %v", got, want)
	}
}

func TestFiles_Raw_StreamsBytesAndHonoursCap(t *testing.T) {
	root := t.TempDir()
	withFileRoot(t, root)
	s, _ := newServer(t, &fakeCaller{}, "secret")
	if err := os.WriteFile(filepath.Join(root, "snippet.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/files/raw?path=snippet.go&token=secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("raw: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "package main\n" {
		t.Fatalf("body=%q, want %q", got, "package main\n")
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("missing content-type header")
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" {
		t.Fatalf("missing cache-control")
	}

	// Over-cap: deliberately set the cap below our test file's size.
	t.Setenv("AGEZT_FILE_ROOT_MAX_BYTES", "4")
	rec = httpJSON(t, s.Handler(), http.MethodGet, "/api/files/raw?path=snippet.go&token=secret", "")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-cap: code=%d, want 413", rec.Code)
	}
}

// TestFiles_Raw_DownloadFilenameIsSanitized guards INJ-003.
//
// The raw download built `attachment; filename="<base>"` by concatenation. A
// filename containing a double quote — legal on Linux and macOS, and an agent
// can create one — closes the quoted string early and lets the rest of the
// name supply attacker-chosen Content-Disposition parameters, so the browser
// saves the file under a spoofed name. The sibling artifact route has always
// run the same value through sanitizeFilename; this one did not.
//
// Response splitting is separately impossible here (Go rewrites CR/LF in
// header values), so the quote breakout is the whole finding.
func TestFiles_Raw_DownloadFilenameIsSanitized(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(`Windows does not permit '"' in a filename, so the breakout cannot be staged here`)
	}
	root := t.TempDir()
	withFileRoot(t, root)
	s, _ := newServer(t, &fakeCaller{}, "secret")

	// A quote closes the quoted-string; the tail then reads as extra
	// Content-Disposition parameters naming a different, executable file.
	const evil = `report".txt"; filename="pwn.sh`
	if err := os.WriteFile(filepath.Join(root, evil), []byte("x\n"), 0o600); err != nil {
		t.Skipf("cannot stage a quoted filename on this filesystem: %v", err)
	}

	rec := httpJSON(t, s.Handler(), http.MethodGet,
		"/api/files/raw?path="+url.QueryEscape(evil)+"&download=1&token=secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("raw: code=%d body=%s", rec.Code, rec.Body.String())
	}

	cd := rec.Header().Get("Content-Disposition")
	if cd == "" {
		t.Fatal("missing Content-Disposition on a download=1 request")
	}

	// The header must still parse as one well-formed disposition carrying a
	// single filename. Unsanitized, it reads as
	//   attachment; filename="report".txt"; filename="pwn.sh"
	// where the quote ends the value early and the tail becomes extra
	// parameters.
	typ, params, err := mime.ParseMediaType(cd)
	if err != nil {
		t.Fatalf("Content-Disposition = %q does not parse: %v — the filename broke out of its quoted string (INJ-003)", cd, err)
	}
	if typ != "attachment" {
		t.Errorf("disposition type = %q, want %q", typ, "attachment")
	}
	if len(params) != 1 {
		t.Errorf("Content-Disposition = %q yielded %d parameters %v, want exactly 1 (filename) — the value broke out (INJ-003)", cd, len(params), params)
	}
	// The whole hostile name must survive as ONE inert filename value, with the
	// quote neutralised rather than honoured as a delimiter.
	if got, want := params["filename"], sanitizeFilename(evil); got != want {
		t.Errorf("filename = %q, want %q (sanitizeFilename output)", got, want)
	}
	if strings.Contains(params["filename"], `"`) {
		t.Errorf("filename %q still contains a raw quote", params["filename"])
	}
	// And the route must reuse the existing helper rather than invent a second policy.
	if want := `attachment; filename="` + sanitizeFilename(evil) + `"`; cd != want {
		t.Errorf("Content-Disposition = %q, want %q", cd, want)
	}
}

func TestFiles_PathTraversalRefused(t *testing.T) {
	root := t.TempDir()
	withFileRoot(t, root)
	s, _ := newServer(t, &fakeCaller{}, "secret")
	// Plant a top-level "secret.txt" OUTSIDE the root that we must never read.
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("OWNED\n"), 0o600); err != nil {
		t.Fatalf("setup outside: %v", err)
	}

	cases := []string{
		"../secret.txt",        // plain traversal
		"foo/../../secret.txt", // nested traversal
		"/etc/passwd",          // absolute
		`C:\\Windows`,          // windows drive-prefix
	}
	for _, p := range cases {
		rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/files/raw?path="+p+"&token=secret", "")
		if rec.Code == http.StatusOK {
			t.Errorf("traversal %q: code=200, want refusal. body=%s", p, rec.Body.String())
		}
	}
	// NUL-byte defence: bypass the URL parser (which would itself reject the
	// path) by hand-crafting a request whose body the handler can read.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw?path=foo%00bar&token=secret", nil)
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("URL-encoded NUL path read OK: body=%s", rec.Body.String())
	}
}

func TestFiles_SymlinkRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics on Windows vary; covered by the resolver tests")
	}
	root := t.TempDir()
	withFileRoot(t, root)
	outsideDir := t.TempDir()
	if err := os.Symlink(filepath.Join(outsideDir, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("OWNED\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	s, _ := newServer(t, &fakeCaller{}, "secret")

	// The tree read must refuse to surface the symlink at all.
	rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/files/tree?path=&token=secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tree root: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"link.txt"`) {
		t.Fatalf("tree listed the symlink: %s", rec.Body.String())
	}

	// A direct raw read must also refuse.
	rec = httpJSON(t, s.Handler(), http.MethodGet, "/api/files/raw?path=link.txt&token=secret", "")
	if rec.Code == http.StatusOK {
		t.Fatalf("symlink raw read returned 200; body=%s", rec.Body.String())
	}
}

// TestFiles_SymlinkDeleteRefused locks in the same defence for the delete
// handler: os.Stat used to follow the link there too, hiding the ModeSymlink
// bit and letting os.Remove/os.RemoveAll chase a target outside the root.
func TestFiles_SymlinkDeleteRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics on Windows vary; covered by the resolver tests")
	}
	root := t.TempDir()
	withFileRoot(t, root)
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "secret.txt")
	if err := os.Symlink(target, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.WriteFile(target, []byte("OWNED\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	s, _ := newServer(t, &fakeCaller{}, "secret")

	// Deleting the in-root symlink must be refused, never chase the target.
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/files/delete?token=secret",
		`{"path":"link.txt"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("symlink delete returned 200; body=%s", rec.Body.String())
	}
	// The out-of-root target must be untouched.
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("out-of-root target was removed via symlink: %v", err)
	}
}

func TestFiles_MkdirRenameDeleteRoundTrip(t *testing.T) {
	root := t.TempDir()
	withFileRoot(t, root)
	s, _ := newServer(t, &fakeCaller{}, "secret")

	// mkdir (with parents=true to create nested dir in one shot)
	rec := httpJSON(t, s.Handler(), http.MethodPost, "/api/files/mkdir?token=secret",
		`{"path":"projects/web","parents":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "web")); err != nil {
		t.Fatalf("projects/web not created: %v", err)
	}

	// rename the empty dir
	rec = httpJSON(t, s.Handler(), http.MethodPost, "/api/files/rename?token=secret",
		`{"from":"projects/web","to":"projects/site"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "site")); err != nil {
		t.Fatalf("renamed dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "web")); !os.IsNotExist(err) {
		t.Fatalf("original dir still present: %v", err)
	}

	// delete (recursive=true works on the empty dir, and future-proofs it)
	rec = httpJSON(t, s.Handler(), http.MethodPost, "/api/files/delete?token=secret",
		`{"path":"projects/site","recursive":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "site")); !os.IsNotExist(err) {
		t.Fatalf("deleted dir still present: %v", err)
	}

	// Non-POST method is refused (CSRF-style defence).
	rec = httpJSON(t, s.Handler(), http.MethodGet, "/api/files/mkdir?token=secret",
		`{"path":"nope"}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/files/mkdir: code=%d, want 405", rec.Code)
	}
}

func TestFiles_DirTraversalOnReadRefusesEscapes(t *testing.T) {
	root := t.TempDir()
	withFileRoot(t, root)
	s, _ := newServer(t, &fakeCaller{}, "secret")

	rec := httpJSON(t, s.Handler(), http.MethodGet, "/api/files/raw?path=..%2F..%2Fetc%2Fpasswd&token=secret", "")
	// URL-decoded path: "../../etc/passwd". The handler must refuse this.
	if rec.Code == http.StatusOK {
		t.Fatalf("URL-encoded traversal succeeded: body=%s", rec.Body.String())
	}
}

// TestFiles_SymlinkedDirectoryRefused is the PATH-001 regression test.
//
// The existing symlink tests all link the FINAL component, which the handlers'
// os.Lstat guards catch. The gap was a symlinked DIRECTORY: lstat only inspects
// the last element and follows every directory component before it, while
// resolveFileRoot was purely lexical — its comment claimed "then resolved
// symlinks" and nothing resolved anything.
//
// So `escape/secret.txt`, where `escape` is a link out of the root, passed the
// string check, passed the final-component lstat (secret.txt is a real file),
// and gave arbitrary read — plus delete and rename, which had no link check at
// all. The most ordinary trigger is not an attacker: it is pointing
// AGEZT_FILE_ROOT at a pnpm project, whose node_modules links into a store
// outside the root.
func TestFiles_SymlinkedDirectoryRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics on Windows vary; covered by the resolver tests")
	}
	root := t.TempDir()
	withFileRoot(t, root)
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("OWNED\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// The link is a DIRECTORY inside the root; the file it exposes is real.
	if err := os.Symlink(outsideDir, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	s, _ := newServer(t, &fakeCaller{}, "secret")

	for _, tc := range []struct {
		name   string
		method string
		url    string
		body   string
	}{
		{"raw read", http.MethodGet, "/api/files/raw?path=escape/secret.txt&token=secret", ""},
		{"tree listing", http.MethodGet, "/api/files/tree?path=escape&token=secret", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httpJSON(t, s.Handler(), tc.method, tc.url, tc.body)
			if rec.Code == http.StatusOK {
				t.Fatalf("traversal through a symlinked directory returned 200; body=%s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "OWNED") {
				t.Fatalf("response leaked the out-of-root file: %s", rec.Body.String())
			}
		})
	}
}

// TestFiles_ResolverAllowsCreationPaths guards the fix from overshooting.
// filepath.EvalSymlinks fails on a path that does not exist, but mkdir and
// first-write legitimately name one. If the resolver refused those, the file
// manager would be unable to create anything — a self-inflicted outage in the
// name of security.
func TestFiles_ResolverAllowsCreationPaths(t *testing.T) {
	root := t.TempDir()
	if err := verifyResolvedWithinRoot(root, filepath.Join(root, "does", "not", "exist", "yet.txt")); err != nil {
		t.Fatalf("a not-yet-created path under the root must be allowed: %v", err)
	}
	if err := verifyResolvedWithinRoot(root, root); err != nil {
		t.Fatalf("the root itself must be allowed: %v", err)
	}
	// And it still refuses a lexically-contained path whose real location is out.
	outside := t.TempDir()
	if err := linkDir(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("cannot create a directory link on this host: %v", err)
	}
	if err := verifyResolvedWithinRoot(root, filepath.Join(root, "link", "x.txt")); err == nil {
		t.Fatal("a path through a linked directory must be refused")
	}
}

// linkDir creates a directory link, falling back to a Windows JUNCTION when
// os.Symlink is refused — creating a symlink on Windows needs Developer Mode or
// elevation, but `mklink /J` needs neither, so a junction is what an ordinary
// Windows user (or a package manager) actually produces.
//
// That distinction is load-bearing rather than cosmetic. Verified against a real
// junction: filepath.EvalSymlinks returns a junction's path UNCHANGED and
// os.Lstat reports it as ModeIrregular, not ModeSymlink — so the first version
// of this fix, which relied on EvalSymlinks alone, passed its POSIX tests and
// still let the traversal through on Windows. Only os.Readlink resolves it.
func linkDir(target, link string) error {
	if err := os.Symlink(target, link); err == nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Symlink(target, link)
	}
	return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
}
