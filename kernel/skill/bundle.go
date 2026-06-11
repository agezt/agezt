// SPDX-License-Identifier: MIT

package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BundleStore holds the on-disk resources that travel with a skill — the
// agentskills.io / ClawHub bundle shape (SPEC-13). A skill is no longer only an
// inline body: it can ship REFERENCE files (extra docs the body points at) and
// SCRIPTS (helpers the agent runs — `scripts/setup.sh` to install a CLI, a
// Python tool, etc.). The SKILL.md body stays small and is injected into
// context; the heavier resources live here and are disclosed progressively —
// the agent lists them (op=files), reads one when relevant (op=read), and runs
// scripts with its existing shell/code_exec tools against the bundle Dir.
//
// Layout: <skillsDir>/bundles/<slug>/<relative-path...>. Bundles are keyed by
// the skill's slug (normalized name), not its content-id, because resources are
// stable across body edits (a new version of "pdf-fill" keeps the same scripts).
//
// A BundleStore is safe for concurrent use: each operation is a self-contained
// filesystem call and writes go through a temp-dir swap (see Write).
type BundleStore struct {
	root string // <skillsDir>/bundles
}

// OpenBundles opens (or creates) the bundle store rooted at <dir>/bundles.
func OpenBundles(dir string) (*BundleStore, error) {
	root := filepath.Join(dir, "bundles")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("skill: mkdir bundles %s: %w", root, err)
	}
	return &BundleStore{root: root}, nil
}

// MaxBundleFile caps a single resource file; MaxBundleTotal caps a whole bundle.
// Bundles are text procedures and helper scripts, not data blobs — generous
// limits that still stop an accidental gigabyte from landing in the skill store.
const (
	MaxBundleFile  = 1 << 20       // 1 MiB per file
	MaxBundleTotal = 8 * (1 << 20) // 8 MiB per bundle
)

// slugify normalizes a skill name into a filesystem-safe directory segment:
// lowercased, trimmed, with any run of non-[a-z0-9_] folded to a single '-'.
// Matches the name-folding ContentID uses so a bundle and its skill agree.
func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	dash := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// cleanRel validates a bundle-relative path and returns it in slash-free OS form.
// It rejects absolute paths and any path that escapes the bundle root (`..`), so
// an imported bundle can never write outside its own directory.
func cleanRel(rel string) (string, error) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "" {
		return "", errors.New("skill: empty bundle path")
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("skill: bundle path %q must be relative", rel)
	}
	cleaned := filepath.Clean(filepath.FromSlash(rel))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("skill: bundle path %q escapes the bundle", rel)
	}
	return cleaned, nil
}

// Write materializes a bundle for the named skill, REPLACING any prior bundle so
// a re-import that drops a file leaves no stale resource behind. The write is
// staged in a sibling temp directory and swapped into place, so a partial or
// failed write never corrupts an existing bundle. Returns the relative paths
// actually written, sorted, in slash form (the stable wire shape).
func (b *BundleStore) Write(name string, files map[string][]byte) ([]string, error) {
	slug := slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("skill: cannot derive a bundle slug from name %q", name)
	}
	if len(files) == 0 {
		return nil, nil
	}
	// Validate everything before touching disk.
	total := 0
	staged := make(map[string][]byte, len(files))
	for rel, data := range files {
		cleaned, err := cleanRel(rel)
		if err != nil {
			return nil, err
		}
		if len(data) > MaxBundleFile {
			return nil, fmt.Errorf("skill: bundle file %q is %d bytes (max %d)", rel, len(data), MaxBundleFile)
		}
		total += len(data)
		if total > MaxBundleTotal {
			return nil, fmt.Errorf("skill: bundle %q exceeds %d bytes total", name, MaxBundleTotal)
		}
		staged[cleaned] = data
	}

	dst := filepath.Join(b.root, slug)
	tmp := dst + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return nil, fmt.Errorf("skill: clear bundle temp: %w", err)
	}
	rels := make([]string, 0, len(staged))
	for cleaned, data := range staged {
		full := filepath.Join(tmp, cleaned)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			os.RemoveAll(tmp)
			return nil, fmt.Errorf("skill: mkdir bundle dir: %w", err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			os.RemoveAll(tmp)
			return nil, fmt.Errorf("skill: write bundle file %q: %w", cleaned, err)
		}
		rels = append(rels, filepath.ToSlash(cleaned))
	}
	// Swap: remove the old bundle, then rename the staged one over it.
	if err := os.RemoveAll(dst); err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("skill: replace bundle: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("skill: commit bundle: %w", err)
	}
	sort.Strings(rels)
	return rels, nil
}

// List returns the bundle's resource paths (sorted, slash form), or nil if the
// skill has no bundle.
func (b *BundleStore) List(name string) ([]string, error) {
	slug := slugify(name)
	if slug == "" {
		return nil, nil
	}
	dir := filepath.Join(b.root, slug)
	var rels []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // no bundle for this skill — not an error
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("skill: list bundle %q: %w", name, err)
	}
	sort.Strings(rels)
	return rels, nil
}

// Read returns the contents of one bundle resource. The relative path is
// validated against escape, so a caller (or a tool the agent drives) can never
// read outside the bundle.
func (b *BundleStore) Read(name, rel string) ([]byte, error) {
	slug := slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("skill: cannot derive a bundle slug from name %q", name)
	}
	cleaned, err := cleanRel(rel)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(b.root, slug, cleaned)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill: bundle resource %q not found in %q", rel, name)
		}
		return nil, fmt.Errorf("skill: read bundle resource: %w", err)
	}
	return data, nil
}

// Dir returns the absolute bundle directory for the named skill — the working
// directory the agent runs the bundle's scripts from. The directory may not yet
// exist (a skill with no resources); callers that need it present should List
// first.
func (b *BundleStore) Dir(name string) string {
	slug := slugify(name)
	if slug == "" {
		return ""
	}
	return filepath.Join(b.root, slug)
}

// Remove deletes a skill's bundle directory (used when a skill is hard-removed).
// Absent bundle is not an error.
func (b *BundleStore) Remove(name string) error {
	slug := slugify(name)
	if slug == "" {
		return nil
	}
	if err := os.RemoveAll(filepath.Join(b.root, slug)); err != nil {
		return fmt.Errorf("skill: remove bundle %q: %w", name, err)
	}
	return nil
}
