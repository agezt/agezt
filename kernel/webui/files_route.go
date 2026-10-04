// SPDX-License-Identifier: MIT

package webui

// Provenance: WebUI files route: types + path resolution + safety layer. Code
//             extracted from files_route.go during the Day-89 god-file split. Public
//             API unchanged.

import "github.com/agezt/agezt/kernel/platform/fileworkspace"

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
	defaultMaxBytes        = 4 * 1024 * 1024
	defaultMaxEntries      = 500
	defaultDirPerm         = 0o700
	defaultFileCap         = 256
	defaultMkdirMaxParents = 8
)

// resolveFileRoot retains the HTTP adapters' unchanged path/error contract.
func (s *Server) resolveFileRoot(rel string) (string, string, string, error) {
	return fileworkspace.Resolve(rel)
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

// handleFileTree lists one directory under the workspace root. Pagination is
// a hard cap, not page-over-page: this is a browse surface, not a search index.
