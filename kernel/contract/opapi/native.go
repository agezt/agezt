// SPDX-License-Identifier: MIT

package opapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

// The console's own endpoints.
//
// The catalogue in http.go binds PATHS TO OPERATIONS: every entry there answers
// a daemon operation. These paths answer the console itself — the SPA shell,
// the login surface, the event feed, the file manager, the voice surface, the
// rollback surface, the workflow hook. They are not operation bindings, so
// they are not in the catalogue; but they ARE part of the console's HTTP
// surface, so they live here, in the same package as the catalogue, rather
// than in a test file nobody but the test can read.
//
// Why this matters for W5.1: the generated TypeScript client publishes one
// path union per route kind. The catalogue gives the operation-bound paths;
// this list gives the console-native ones. Both come from the same package,
// so the client and the console's own route guard read the same source.
//
// Before this slice the list held PATHS ONLY, and the console registered each
// endpoint by hand — writing the path a second time, and choosing the method,
// the tier and the mutation flag in a second place. The path was written twice;
// the verb was written once but nowhere the client could see it, so the
// published client accepted a GET for /api/login and a POST for /api/authmeta,
// both of which the console answers 405.
//
// A native route is now a PROMISE with four parts: the path, the method, the
// tier and the mutation flag. The console binds its handler by NAME (never by
// path), so the path is written once — here — and the console derives its
// registration from the list. The generated client splits the published
// native paths by verb, so the wrong verb is a compile error rather than a
// 405 at runtime.

// NativeRouteName identifies one of the console's own endpoints. The console
// binds its handler by this name, so a name in the contract and a handler in
// the console are joined by a key that cannot be a path.
type NativeRouteName string

// The names of the console's own endpoints.
const (
	NativeShell         NativeRouteName = "shell"
	NativeAuthMeta      NativeRouteName = "authmeta"
	NativeLogin         NativeRouteName = "login"
	NativeLogout        NativeRouteName = "logout"
	NativeAssets        NativeRouteName = "assets"
	NativeFavicon       NativeRouteName = "favicon"
	NativeEvents        NativeRouteName = "events"
	NativeRollbackList  NativeRouteName = "rollback_checkpoints"
	NativeRollbackApply NativeRouteName = "rollback_apply"
	NativeSSEToken      NativeRouteName = "sse_token"
	NativeVoiceStatus   NativeRouteName = "voice_status"
	NativeTranscribe    NativeRouteName = "transcribe"
	NativeTTS           NativeRouteName = "tts"
	NativeArtifactRaw   NativeRouteName = "artifact_raw"
	NativeFilesTree     NativeRouteName = "files_tree"
	NativeFilesRaw      NativeRouteName = "files_raw"
	NativeFilesMkdir    NativeRouteName = "files_mkdir"
	NativeFilesRename   NativeRouteName = "files_rename"
	NativeFilesDelete   NativeRouteName = "files_delete"
	NativeWorkflowHook  NativeRouteName = "workflow_hook"
	NativeOAuthCallback NativeRouteName = "oauth_callback"
)

// NativeRoute is one of the console's own endpoints: the path the console
// answers at, the method it answers with, whether it is token-free, and
// whether it changes state.
//
// Mutation is NOT derived from the method: the OAuth callback is a GET that
// stores a channel token, so the flag has to be stated.
//
// Body is the JSON Schema of the request body this endpoint takes — empty
// when it takes none. It is the console's own half of the request side: the
// console's operation-bound routes get their body shapes from the daemon's
// operation specs, but these endpoints are not operation bindings, so their
// shapes live HERE, next to the path they are served at. The console checks
// an arriving body against it before the handler reads it (kernel/webui), and
// refuses a mistyped one in the operation registry's words.
//
// Two rules keep the schema the registry's shape: it carries no `required`
// list (the registry's door checks TYPES; presence is the handler's business)
// and it allows a field it does not name (a schema is the stable subset of a
// body).
type NativeRoute struct {
	Name     NativeRouteName
	Path     string
	Method   string
	Public   bool
	Mutation bool
	Body     json.RawMessage
}

// ConsoleNativeBodySchema returns the request-body schema of one console-native
// endpoint. An endpoint that takes no body answers false.
func ConsoleNativeBodySchema(name NativeRouteName) (json.RawMessage, bool) {
	for _, route := range consoleNativeRoutes {
		if route.Name == name && len(route.Body) > 0 {
			return route.Body, true
		}
	}
	return nil, false
}

// IsWrite reports whether the route answers a state-changing verb. It is the
// one question the generated client asks of a console-native path: a read
// native path may only be fetched, a write native path may only be posted.
func (r NativeRoute) IsWrite() bool {
	for _, candidate := range strings.Split(r.Method, ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), http.MethodPost) {
			return true
		}
	}
	return false
}

// ConsoleNativeRoutes returns the console's own endpoints.
func ConsoleNativeRoutes() []NativeRoute {
	routes := make([]NativeRoute, len(consoleNativeRoutes))
	copy(routes, consoleNativeRoutes)
	return routes
}

// ConsoleNativePath returns the path of one console-native endpoint. A name
// the list does not carry is a programmer error, so it panics rather than
// answering with a path nobody serves.
func ConsoleNativePath(name NativeRouteName) string {
	for _, route := range consoleNativeRoutes {
		if route.Name == name {
			return route.Path
		}
	}
	panic("opapi: no console-native endpoint named " + string(name))
}

// consoleNativeRoutes is the console's own endpoint list: one entry per
// endpoint the console answers that is not an operation binding.
var consoleNativeRoutes = []NativeRoute{
	// The SPA shell loads with the token OR — when a console password is
	// configured (M933) — with no credential at all, so a token-less visitor
	// gets the password login screen instead of a bare 401. The shell carries
	// no data; every data route stays protected by route policy.
	{Name: NativeShell, Path: "/", Method: "GET", Public: true},
	// Auth surface (M817/M933): probe + login/logout are token-FREE — they are
	// how a token-less browser gets in. authmeta leaks only "is there a
	// password gate" (no data); login is bounded by the failed-attempt lockout.
	{Name: NativeAuthMeta, Path: "/api/authmeta", Method: "GET", Public: true},
	{Name: NativeLogin, Path: "/api/login", Method: "POST", Public: true, Mutation: true,
		Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"password":{"type":"string"}}}`)},
	{Name: NativeLogout, Path: "/api/logout", Method: "POST", Public: true, Mutation: true},
	// The hashed bundle and favicon are PUBLIC: the browser loads them as
	// subresources of index.html and cannot attach the ?token= (only fetch /
	// EventSource, which the SPA controls, can). They carry no secrets (compiled
	// UI code). The data surfaces — /events and every /api/* — stay token-gated,
	// so an unauthenticated visitor gets the shell but no data.
	{Name: NativeAssets, Path: "/assets/", Method: "GET", Public: true},
	{Name: NativeFavicon, Path: "/favicon.ico", Method: "GET", Public: true},
	{Name: NativeEvents, Path: "/events", Method: "GET"},
	{Name: NativeRollbackList, Path: "/api/rollback/checkpoints", Method: "GET"},
	{Name: NativeRollbackApply, Path: "/api/rollback/apply", Method: "POST", Mutation: true,
		Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"}}}`)},
	// SSE token endpoint (VULN query-string-token fix): returns the ephemeral
	// SSE-only token the SPA embeds in the EventSource URL (?st=...) instead of
	// reusing the main console token. Requires valid WebUI data authorization.
	{Name: NativeSSEToken, Path: "/api/sse-token", Method: "GET,OPTIONS"},
	// Provider-free speech readiness probe. Unlike the STT/TTS action routes,
	// this never uploads dummy audio or synthesizes throwaway speech.
	{Name: NativeVoiceStatus, Path: "/api/voice/status", Method: "GET"},
	{Name: NativeTranscribe, Path: "/api/transcribe", Method: "POST", Mutation: true},
	// Text-to-speech for the console Voice Mode (M998): POST {"text":…} and the
	// server streams synthesized audio from the configured TTS backend.
	{Name: NativeTTS, Path: "/api/tts", Method: "POST", Mutation: true,
		Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"text":{"type":"string"}}}`)},
	// Binary artifact serving (M822): streams the raw bytes for a content ref with
	// a sanitized Content-Type, so an <img src> / download link can render stored
	// images and files. Proxies opapi.ArtifactGet (which re-verifies the bytes).
	{Name: NativeArtifactRaw, Path: "/api/artifact/raw", Method: "GET"},
	// File Manager routes (M1017): live filesystem under the configured
	// `AGEZT_FILE_ROOT` (default `~/agezt/workspace`). Every endpoint here
	// path-traversal-guards before touching the OS — see files_route.go for
	// the chokepoint.
	{Name: NativeFilesTree, Path: "/api/files/tree", Method: "GET"},
	{Name: NativeFilesRaw, Path: "/api/files/raw", Method: "GET"},
	{Name: NativeFilesMkdir, Path: "/api/files/mkdir", Method: "POST", Mutation: true,
		Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"path":{"type":"string"},"parents":{"type":"boolean"}}}`)},
	{Name: NativeFilesRename, Path: "/api/files/rename", Method: "POST", Mutation: true,
		Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"from":{"type":"string"},"to":{"type":"string"}}}`)},
	{Name: NativeFilesDelete, Path: "/api/files/delete", Method: "POST", Mutation: true,
		Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"path":{"type":"string"},"recursive":{"type":"boolean"}}}`)},
	// Workflow webhooks (M809): the ONE deliberately console-token-free
	// path. Authentication is the per-workflow secret, verified by the
	// control plane (constant-time); all this handler can ever do is ask
	// "fire workflow <name>" — no reads, no other writes, uniform refusals.
	{Name: NativeWorkflowHook, Path: "/hooks/", Method: "POST", Public: true, Mutation: true},
	// Channel OAuth redirect target (Phase 4). Public (no console token): the
	// provider redirects the operator's browser here with ?code&state. Security
	// rests on the unguessable state minted by /api/channel/oauth/start. It is
	// a GET that STORES a token, so it is a mutation.
	{Name: NativeOAuthCallback, Path: "/oauth/callback", Method: "GET", Public: true, Mutation: true},
}
