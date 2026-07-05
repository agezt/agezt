// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File Manager routes (M1017). The frontend's Files workspace talks to a
// live tree + raw bytes under a configurable root, defaulting to
// `~/agezt/workspace`. Every endpoint rejects `..`, absolute paths, and
// symlink escapes against the configured root. Auth is gated by the same
// `s.auth(...)` wrapper the artifact route uses; we deliberately do NOT
// route the reads through `controlplane.Cmd*` because the operation is
// owned by the webui gateway, not the daemon's command surface.
//
// Configuration:
//   AGEZT_FILE_ROOT — directory the routes serve. Default: ~/agezt/workspace.
//                     Created (with 0700 perms) on first access if missing.
//   AGEZT_FILE_ROOT_MAX_BYTES — cap on raw reads (default 4 MiB).
//   AGEZT_FILE_ROOT_MAX_ENTRIES — cap on a single tree response (default 500).

const (
	defaultFileRoot        = "agezt/workspace"
	defaultMaxBytes        = 4 * 1024 * 1024
	defaultMaxEntries      = 500
	defaultDirPerm         = 0o700
	defaultFileCap         = 256
	defaultMkdirMaxParents = 8
)

// fileManagerRoot returns the configured workspace root, creating it on first
// access if missing. A leading "~/" expands to the operator's home directory.
// We do not call into kernel/paths for "~/": that's the OS's job.
func (s *Server) fileManagerRoot() (string, error) {
	raw := strings.TrimSpace(os.Getenv("AGEZT_FILE_ROOT"))
	if raw == "" {
		home, herr := os.UserHomeDir()
		if herr != nil || home == "" {
			return "", fmt.Errorf("AGEZT_FILE_ROOT unset and no home directory")
		}
		raw = filepath.Join(home, defaultFileRoot)
	}
	if strings.HasPrefix(raw, "~/") {
		home, herr := os.UserHomeDir()
		if herr != nil || home == "" {
			return "", fmt.Errorf("AGEZT_FILE_ROOT expands ~ but no home directory")
		}
		raw = filepath.Join(home, strings.TrimPrefix(raw, "~/"))
	} else if raw == "~" {
		home, herr := os.UserHomeDir()
		if herr != nil || home == "" {
			return "", fmt.Errorf("AGEZT_FILE_ROOT expands ~ but no home directory")
		}
		raw = home
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	// Create with restrictive perms on first access. MkdirAll is a no-op when
	// the directory already exists, so this is safe to call on every request.
	if err := os.MkdirAll(abs, defaultDirPerm); err != nil {
		return "", err
	}
	return abs, nil
}

// fileNode mirrors frontend/src/lib/files.ts FileNode. Field names + JSON
// casing are part of the cross-package contract; renaming here means the
// UI must move too.
type fileNode struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"` // "dir" | "file"
	Size       int64  `json:"size,omitempty"`
	ModifiedMS int64  `json:"modified_ms,omitempty"`
}

// fileTreeResponse mirrors frontend/src/lib/files.ts FileTreeResponse.
type fileTreeResponse struct {
	Root  string     `json:"root"`
	Nodes []fileNode `json:"nodes"`
}

// resolveFileRoot ensures the user-supplied relative path stays inside the
// configured root and returns the absolute filesystem path + the relative
// POSIX path the client passed (for echoing back into FileNode.Path). It
// is the single chokepoint for path-traversal defence: every handler below
// funnels through here.
//
//	rootAbs  := <configured root absolute path>
//	relPosix := the user-supplied POSIX path, cleaned
//	target   := filepath.Join(rootAbs, relPosix) then resolved symlinks
//
// Anywhere along that walk that escapes `rootAbs` is a refusal with a 400.
func (s *Server) resolveFileRoot(rel string) (rootAbs string, targetAbs string, relPosix string, err error) {
	rootAbs, err = s.fileManagerRoot()
	if err != nil {
		return "", "", "", err
	}
	cleaned, perr := sanitizeRelativePath(rel)
	if perr != nil {
		return "", "", "", perr
	}
	relPosix = cleaned
	targetAbs = filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(relPosix)))
	// Both sides clean. If the cleaned target is the root, allow. Otherwise
	// it must live under rootAbs — both sides via Lexical prefix, not
	// `strings.HasPrefix` (so `/var/foo` doesn't match `/var/foobar`).
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return "", "", "", fmt.Errorf("path escapes root")
	}
	return rootAbs, targetAbs, relPosix, nil
}

// sanitizeRelativePath rejects empty-after-clean, absolute paths, NUL bytes,
// and `..` segments. The result is always a forward-slash POSIX path that
// can flow back into JSON without further escaping.
func sanitizeRelativePath(p string) (string, error) {
	cleaned := strings.TrimSpace(p)
	// Reject NUL early — filepath/URL parsers stop at NUL, which would let an
	// attacker craft paths that looke different to the UI vs. the OS.
	if strings.ContainsRune(cleaned, '\x00') {
		return "", fmt.Errorf("path contains NUL")
	}
	// Reject Windows drive letters and leading slashes.
	if cleaned != "" && (cleaned[0] == '/' || cleaned[0] == '\\') {
		return "", fmt.Errorf("path must be relative")
	}
	if cleaned != "" && len(cleaned) >= 2 && cleaned[1] == ':' {
		return "", fmt.Errorf("path must be relative")
	}
	// filepath.Clean uses the OS separator; we convert to POSIX for the
	// client. Doubled slashes and `.` segments collapse here.
	cleaned = filepath.ToSlash(filepath.Clean(filepath.FromSlash(cleaned)))
	// After Clean, "" means the user asked for the root → fine.
	segs := strings.Split(cleaned, "/")
	for _, s := range segs {
		if s == ".." {
			return "", fmt.Errorf("path contains '..'")
		}
	}
	return cleaned, nil
}

// handleFileTree lists one directory under the workspace root. Pagination is
// a hard cap, not page-over-page: this is a browse surface, not a search index.
func (s *Server) handleFileTree(w http.ResponseWriter, r *http.Request) {
	rootAbs, targetAbs, rel, err := s.resolveFileRoot(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(targetAbs)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !info.IsDir() {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	}
	entries, err := os.ReadDir(targetAbs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cap := defaultMaxEntries
	if v := os.Getenv("AGEZT_FILE_ROOT_MAX_ENTRIES"); v != "" {
		var n int
		_, sErr := fmt.Sscanf(v, "%d", &n)
		if sErr == nil && n > 0 {
			cap = n
		}
	}
	if len(entries) > cap {
		entries = entries[:cap]
	}
	sort.Slice(entries, func(i, j int) bool {
		ai, bi := entries[i], entries[j]
		if ai.IsDir() != bi.IsDir() {
			return ai.IsDir() // directories first
		}
		return strings.ToLower(ai.Name()) < strings.ToLower(bi.Name())
	})
	nodes := make([]fileNode, 0, len(entries))
	for _, e := range entries {
		full := filepath.Join(targetAbs, e.Name())
		fi, ferr := e.Info()
		var size int64
		var modMS int64
		if ferr == nil && !e.IsDir() {
			size = fi.Size()
			modMS = fi.ModTime().UnixMilli()
		} else if ferr == nil {
			modMS = fi.ModTime().UnixMilli()
		}
		// Belt + braces: if a symlink slipped through the readdir call, refuse
		// to surface it to the client. The client only sees regular files /
		// directories that we could resolve without chasing a link.
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		childRel := filepath.ToSlash(filepath.Join(rel, e.Name()))
		nodes = append(nodes, fileNode{
			Name:       e.Name(),
			Path:       childRel,
			Type:       typeOf(e),
			Size:       size,
			ModifiedMS: modMS,
		})
		_ = full
	}
	resp := fileTreeResponse{
		Root:  rel,
		Nodes: nodes,
	}
	writeJSON(w, http.StatusOK, resp)
	_ = rootAbs
}

// handleFileRaw streams a file's bytes. The size cap is a defensive bound
// — a chat mention that points at a multi-gigabyte file is almost certainly
// an accident, and we want the daemon to refuse instead of OOM.
func (s *Server) handleFileRaw(w http.ResponseWriter, r *http.Request) {
	_, targetAbs, _, err := s.resolveFileRoot(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Lstat, not Stat: Stat follows the symlink and loses the ModeSymlink bit,
	// which would let an in-root link to an out-of-root file sail past the
	// symlink guard below and stream bytes from outside the workspace.
	info, err := os.Lstat(targetAbs)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		http.Error(w, "is a directory", http.StatusBadRequest)
		return
	}
	// Refuse symlinks outright — chasing them is the easiest way to escape
	// the configured root via a `..`-style alias from inside the workspace.
	if info.Mode()&os.ModeSymlink != 0 {
		http.Error(w, "symlinks are not served", http.StatusForbidden)
		return
	}
	cap := int64(defaultMaxBytes)
	if v := os.Getenv("AGEZT_FILE_ROOT_MAX_BYTES"); v != "" {
		fmt.Sscanf(v, "%d", &cap)
	}
	if info.Size() > cap {
		http.Error(w, fmt.Sprintf("file exceeds %d-byte cap", cap), http.StatusRequestEntityTooLarge)
		return
	}
	download := r.URL.Query().Get("download") == "1"
	name := filepath.Base(targetAbs)
	if download {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	f, ferr := os.Open(targetAbs)
	if ferr != nil {
		return
	}
	defer f.Close()
	_, _ = io.Copy(w, f)
}

// handleFileMkdir creates a directory under the workspace root. POST body
// shape (form-urlencoded or JSON): {path, parents?}. `parents` lets callers
// create intermediate directories the same way `mkdir -p` does — required
// because the UI's "new folder" affordance doesn't always know the depth.
func (s *Server) handleFileMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, ok := readJSONBody(w, r, defaultFileCap)
	if !ok {
		return
	}
	rel, _ := body["path"].(string)
	parents, _ := body["parents"].(bool)
	_, targetAbs, _, err := s.resolveFileRoot(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if parents {
		if err := os.MkdirAll(targetAbs, defaultDirPerm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := os.Mkdir(targetAbs, defaultDirPerm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": filepath.ToSlash(rel)})
}

// handleFileRename moves/renames a path within the workspace root. POST body:
// {from, to}. Refuses if either side escapes the root.
func (s *Server) handleFileRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, ok := readJSONBody(w, r, defaultFileCap)
	if !ok {
		return
	}
	from, _ := body["from"].(string)
	to, _ := body["to"].(string)
	if from == "" || to == "" {
		http.Error(w, "from and to required", http.StatusBadRequest)
		return
	}
	_, fromAbs, _, err := s.resolveFileRoot(from)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, toAbs, _, err := s.resolveFileRoot(to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "from": filepath.ToSlash(from), "to": filepath.ToSlash(to)})
}

// handleFileDelete removes a file or empty directory. POST body: {path}.
// Non-empty directories require recursive=true to avoid surprising the
// operator with a half-deleted subtree.
func (s *Server) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, ok := readJSONBody(w, r, defaultFileCap)
	if !ok {
		return
	}
	rel, _ := body["path"].(string)
	recursive, _ := body["recursive"].(bool)
	_, targetAbs, _, err := s.resolveFileRoot(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Lstat, not Stat: Stat resolves the link, hiding the ModeSymlink bit and
	// defeating the symlink guard below — an in-root link could then let
	// os.Remove/os.RemoveAll chase a target outside the workspace.
	info, err := os.Lstat(targetAbs)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Belt + braces: even when recursive=true, refuse symlinks. A symlink
	// inside the root can point outside, and `os.Remove` happily chases it.
	if info.Mode()&os.ModeSymlink != 0 {
		http.Error(w, "symlinks are not served", http.StatusForbidden)
		return
	}
	if info.IsDir() && recursive {
		if err := os.RemoveAll(targetAbs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := os.Remove(targetAbs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": filepath.ToSlash(rel)})
}

// typeOf returns "dir" or "file" for an os.DirEntry — keeping the JSON
// contract exactly two-value and lowercase.
func typeOf(e os.DirEntry) string {
	if e.IsDir() {
		return "dir"
	}
	return "file"
}

// readJSONBody decodes a JSON body up to a small cap. It writes a 4xx
// response and returns ok=false when the body is missing, malformed, or
// over the cap; the caller should just return in those cases.
func readJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64) (map[string]any, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer r.Body.Close()
	var out map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&out); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	return out, true
}
