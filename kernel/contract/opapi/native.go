// SPDX-License-Identifier: MIT

package opapi

import "sort"

// The console's own endpoints.
//
// The catalogue in http.go binds PATHS TO OPERATIONS: every entry there answers
// a daemon operation. These paths answer the console itself — the SPA shell,
// the login surface, the event feed, the file manager, the voice surface, the
// rollback surface, the workflow hook. They are not operation bindings, so
// they are not in the catalogue; but they ARE part of the console's HTTP
// surface, so their NAMES live here, in the same package as the catalogue,
// rather than in a test file nobody but the test can read.
//
// Why this matters for W5.1: the generated TypeScript client publishes one
// path union per route kind. The catalogue gives the operation-bound paths;
// this list gives the console-native ones. Both come from the same package,
// so the client and the console's own route guard read the same source.
//
// A native path is registered by the console as a handler, not derived from
// the catalogue, so the list here is a NAME list, not a binding list. The
// console's route guard test holds both directions: every native path in this
// list is served, and no console path outside this list and the catalogue is
// served.

// ConsoleNativePaths returns the console's own endpoints, sorted.
func ConsoleNativePaths() []string {
	paths := make([]string, len(consoleNativePaths))
	copy(paths, consoleNativePaths)
	sort.Strings(paths)
	return paths
}

// consoleNativePaths is the console's own endpoint list.
var consoleNativePaths = []string{
	"/",
	"/api/artifact/raw",
	"/api/authmeta",
	"/api/files/delete",
	"/api/files/mkdir",
	"/api/files/raw",
	"/api/files/rename",
	"/api/files/tree",
	"/api/login",
	"/api/logout",
	"/api/rollback/apply",
	"/api/rollback/checkpoints",
	"/api/sse-token",
	"/api/transcribe",
	"/api/tts",
	"/api/voice/status",
	"/assets/",
	"/events",
	"/favicon.ico",
	"/hooks/",
	"/oauth/callback",
}
