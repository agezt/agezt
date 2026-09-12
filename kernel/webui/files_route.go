// SPDX-License-Identifier: MIT

// WebUI files route: types + path resolution + safety layer.
// Code extracted from files_route.go during the Day-89 god-file split.
// Public API unchanged.
package webui


import (
	"fmt"
	"os"
	"strings"

	"path/filepath"
)


// File Manager routes (M1017). The frontend's Files workspace talks to a
// live tree + raw bytes under a configurable root, defaulting to
// `~/agezt/workspace`. Every endpoint rejects `..`, absolute paths, and
// symlink escapes against the configured root. Auth is gated by the same
// protected route policy the artifact route uses; we deliberately do NOT route
// the reads through `controlplane.Cmd*` because the operation is owned by the
// webui gateway, not the daemon's command surface.
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
	// Lexical containment constrains the STRING, not the filesystem (PATH-001,
	// 2026-08-12). The comment above promised "then resolved symlinks" and
	// nothing resolved anything: a single symlinked DIRECTORY inside the root
	// pointed anywhere, and every lexical check still passed. The handlers'
	// os.Lstat guards only the FINAL component — lstat happily follows every
	// directory component before it — so that gave arbitrary read, delete and
	// rename outside the root. The most ordinary trigger is pointing
	// AGEZT_FILE_ROOT (operator-settable from the console) at a pnpm project,
	// whose node_modules is a forest of links into a store outside the root.
	if err := verifyResolvedWithinRoot(rootAbs, targetAbs); err != nil {
		return "", "", "", err
	}
	return rootAbs, targetAbs, relPosix, nil
}

// verifyResolvedWithinRoot resolves symlinks in target and confirms the real
// path still lives under the real root.
//
// It deliberately does NOT rewrite the caller's target: handlers keep operating
// on the lexical path so their existing final-component O_NOFOLLOW / Lstat
// guards behave exactly as before. This is a check, not a substitution.
func verifyResolvedWithinRoot(rootAbs, targetAbs string) error {
	// Resolve the root too. A root reached THROUGH a symlink is normal — macOS
	// /var → /private/var, a home directory on a linked volume — and comparing
	// a resolved target against an unresolved root would refuse every path
	// under it.
	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}

	// EvalSymlinks requires the path to exist, but mkdir and first-write
	// legitimately name something that does not yet. Walk up to the deepest
	// EXISTING ancestor, resolve that, and re-attach the not-yet-created tail:
	// the tail cannot be a symlink if it does not exist, so resolving the
	// ancestor is sufficient.
	probe := targetAbs
	var tail []string
	for {
		real, rerr := filepath.EvalSymlinks(probe)
		if rerr == nil {
			resolved := filepath.Join(append([]string{real}, tail...)...)
			if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+string(os.PathSeparator)) {
				return fmt.Errorf("path escapes root")
			}
			break
		}
		if !os.IsNotExist(rerr) {
			return fmt.Errorf("resolve path: %w", rerr)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			// Walked to the filesystem root without finding anything that
			// exists. Refuse rather than guess.
			return fmt.Errorf("path escapes root")
		}
		tail = append([]string{filepath.Base(probe)}, tail...)
		probe = parent
	}
	return verifyNoEscapingLinks(rootAbs, realRoot, targetAbs)
}

// verifyNoEscapingLinks walks every component under the root and refuses any
// reparse point whose target leaves it.
//
// EvalSymlinks alone is NOT enough on Windows, which I established by pointing a
// real directory junction at an out-of-root directory and watching the check
// pass: EvalSymlinks returns a junction's path UNCHANGED, and os.Lstat reports
// it as ModeIrregular rather than ModeSymlink, so both the resolver and a
// mode&ModeSymlink test miss it entirely. os.Readlink does resolve it.
//
// POSIX symlinks are already handled by the EvalSymlinks pass above; this walk
// is what closes junctions, and it costs one Readlink per component.
func verifyNoEscapingLinks(rootAbs, realRoot, targetAbs string) error {
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("path escapes root")
	}
	if rel == "." {
		return nil
	}
	cur := rootAbs
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		dest, lerr := os.Readlink(cur)
		if lerr != nil {
			// Not a link, or does not exist yet. Either way nothing to follow.
			continue
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(cur), dest)
		}
		dest = filepath.Clean(dest)
		if dest != realRoot && !strings.HasPrefix(dest, realRoot+string(os.PathSeparator)) {
			return fmt.Errorf("path escapes root via a link at %q", part)
		}
		// Keep walking from where the link actually lands, so a chain of links
		// is followed rather than only its first hop.
		cur = dest
	}
	return nil
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
