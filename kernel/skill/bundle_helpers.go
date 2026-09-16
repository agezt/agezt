// SPDX-License-Identifier: MIT
//
// kernel/skill bundle-path helpers (slugify, cleanRel, resolveSymlinks).
// Extracted from bundle.go during Day 211 god-file refactor (#92).
// Public API unchanged.
package skill

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

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
func resolveSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
