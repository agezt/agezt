// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
