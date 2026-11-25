// SPDX-License-Identifier: MIT

package webui

// Provenance: files_route_helpers.go holds the tiny format helper used by the
//             /api/v1/files route handlers: typeOf (sort-aware dir-entry label).
//             The handlers themselves live in files_route_handlers.go, and the
//             body they read goes through the console's request door
//             (request_door.go).
//             Carved out during the Day-89 god-file split. Public API unchanged.

import (
	"os"
)

func typeOf(e os.DirEntry) string {
	if e.IsDir() {
		return "dir"
	}
	return "file"
}
