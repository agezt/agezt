// SPDX-License-Identifier: MIT
//
// kernel/skill BundleStore I/O methods (Write, List, Read, Dir, Remove).
// Extracted from bundle.go during Day 211 god-file refactor (#92).
// Public API unchanged.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
	// Resolve symlinks before checking containment — os.ReadFile follows them
	// silently at the kernel level, bypassing cleanRel's lexical guard.
	resolved, err := resolveSymlinks(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill: bundle resource %q not found in %q", rel, name)
		}
		return nil, fmt.Errorf("skill: illegal file path: %w", err)
	}
	// Verify the resolved path is still inside the bundle root.
	if !strings.HasPrefix(resolved, b.root+string(filepath.Separator)) {
		return nil, fmt.Errorf("skill: resolved path %q escapes the bundle root %q", resolved, b.root)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill: bundle resource %q not found in %q", rel, name)
		}
		return nil, fmt.Errorf("skill: read bundle resource: %w", err)
	}
	return data, nil
}
func (b *BundleStore) Dir(name string) string {
	slug := slugify(name)
	if slug == "" {
		return ""
	}
	return filepath.Join(b.root, slug)
}
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
