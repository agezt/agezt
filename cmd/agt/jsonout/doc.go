// SPDX-License-Identifier: MIT

// Package jsonout is the pretty-print-to-JSON helper every
// `--json` command needs. It is the canonical home of what used
// to be the 6-line `encodeJSON(w, v) int` helper living at the
// bottom of cmd/agt/memory.go. The function is trivial — `enc :=
// json.NewEncoder(w); enc.SetIndent("", "  "); enc.Encode(v)` —
// but it was in the wrong place: 143 call sites in 48 files
// across cmd/agt all want it, and now that the package is here
// any future sub-package that needs pretty JSON for its `--json`
// flag imports it directly without joining the 143-site
// rewrite pool.
//
// The function returns 0 because every caller is `return
// encodeJSON(stdout, v)` from a top-level command function whose
// return type is fixed by the dispatcher signature. Returning
// 0 (success) keeps that signature stable; a future feature
// that needs to surface encoder errors can change the return
// type without touching any of the 143 callers.
package jsonout
