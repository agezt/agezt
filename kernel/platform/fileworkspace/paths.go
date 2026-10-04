// SPDX-License-Identifier: MIT

// Package fileworkspace owns console workspace path checks and filesystem
// primitives. Callers own authorization and journal admission.
package fileworkspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultFileRoot = "agezt/workspace"
	defaultDirPerm  = 0o700
)

// Root returns the configured workspace root, creating it on first
// access if missing. A leading "~/" expands to the operator's home directory.
// We do not call into kernel/paths for "~/": that's the OS's job.
func Root() (string, error) {
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

// Resolve ensures the user-supplied relative path stays inside the
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
func Resolve(rel string) (rootAbs string, targetAbs string, relPosix string, err error) {
	rootAbs, err = Root()
	if err != nil {
		return "", "", "", err
	}
	cleaned, perr := SanitizeRelativePath(rel)
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
	if err := VerifyResolvedWithinRoot(rootAbs, targetAbs); err != nil {
		return "", "", "", err
	}
	return rootAbs, targetAbs, relPosix, nil
}

// VerifyResolvedWithinRoot resolves symlinks in target and confirms the real
// path still lives under the real root.
//
// It deliberately does NOT rewrite the caller's target: handlers keep operating
// on the lexical path so their existing final-component O_NOFOLLOW / Lstat
// guards behave exactly as before. This is a check, not a substitution.
func VerifyResolvedWithinRoot(rootAbs, targetAbs string) error {
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

// SanitizeRelativePath rejects empty-after-clean, absolute paths, NUL bytes,
// and `..` segments. The result is always a forward-slash POSIX path that
// can flow back into JSON without further escaping.
func SanitizeRelativePath(p string) (string, error) {
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
