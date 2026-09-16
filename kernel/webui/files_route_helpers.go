// SPDX-License-Identifier: MIT

// files_route_helpers.go holds the tiny format helpers used
// by the /api/v1/files route handlers: typeOf (sort-aware
// dir-entry label) and readJSONBody (cap-bounded JSON
// decoder that surfaces a clean error to the caller). The
// handlers themselves live in files_route_handlers.go.
// Carved out during the Day-89 god-file split. Public API
// unchanged.
package webui

import (
	"encoding/json"
	"net/http"
	"os"
)

func typeOf(e os.DirEntry) string {
	if e.IsDir() {
		return "dir"
	}
	return "file"
}

// readJSONBody decodes a JSON body whose route-level cap has already been
// applied by Handler. It writes a 4xx response and returns ok=false when the
// body is missing, malformed, or over the cap; the caller should just return.
func readJSONBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	defer r.Body.Close()
	var out map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&out); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	return out, true
}
