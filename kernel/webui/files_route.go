// SPDX-License-Identifier: MIT

package webui

import "github.com/agezt/agezt/kernel/contract/consoleapi"

// Provenance: WebUI files route: types + path resolution + safety layer. Code
//             extracted from files_route.go during the Day-89 god-file split. Public
//             API unchanged.

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
	defaultFileCap         = 256
	defaultMkdirMaxParents = 8
)

// fileNode is the console's file-tree row: the CONTRACT's row, by alias, so
// a second spelling of it cannot grow here. The field names and their JSON
// casing live in kernel/contract/consoleapi.
type fileNode = consoleapi.FileNode

// fileTreeResponse is the console's file-tree answer: the contract's row, by
// alias, for the same reason.
type fileTreeResponse = consoleapi.FileTree

// handleFileTree lists one directory under the workspace root. Pagination is
// a hard cap, not page-over-page: this is a browse surface, not a search index.
