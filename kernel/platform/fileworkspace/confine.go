// SPDX-License-Identifier: MIT

package fileworkspace

import (
	"path/filepath"
	"strings"
)

// ConfineUnder joins an untrusted relative path to root and confirms the
// result stays inside root, rejecting "..", absolute paths, NUL bytes and
// Windows drive-relative escapes. The check is lexical: callers that then open
// the path must resolve links themselves. It returns the cleaned path.
func ConfineUnder(root, rel string) (string, bool) {
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "\x00") {
		return "", false
	}
	full := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	cleanRoot := filepath.Clean(root)
	if full != cleanRoot && !strings.HasPrefix(full, cleanRoot+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}
