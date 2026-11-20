// SPDX-License-Identifier: MIT

package webui

// Server route registry: the single source of truth for which URL maps
// to which read/write route. Carved out of webui.go during the Day 26
// god file split #3 so the main file can focus on Server type +
// constructors.

import (
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/httpserver"
	"net/http"
	"time"
)

// jsonBodyMax caps a Flow Studio request body. A generated plan is a few KiB;
// 1 MiB is far above any legitimate plan or intent and bounds memory per call.
const jsonBodyMax = 1 << 20

// planRunTimeout bounds an in-UI plan run. Plans can legitimately take minutes
// (each loop node is a full agent run), so this is generous — far longer than
// the 5s read-panel timeout. The browser sees progress live on the SSE feed
// regardless; this only bounds how long the connection is held open.
const planRunTimeout = 30 * time.Minute

func (s *Server) routeRegistry() *httpserver.Router {
	router := httpserver.NewRouter(httpserver.Authenticator{
		RequestAuthorize: func(r *http.Request, _ kernelauth.Tier) bool {
			return s.authorized(r)
		},
	}, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	publicRead := httpserver.RouteOpts{Tier: kernelauth.TierPublic, Method: http.MethodGet}
	publicMutation := httpserver.RouteOpts{Tier: kernelauth.TierPublic, Method: http.MethodPost, Mutation: true}
	protectedRead := httpserver.RouteOpts{Tier: kernelauth.TierUser, Method: http.MethodGet}
	proxyRead := protectedRead
	proxyRead.Timeout = 5 * time.Second
	protectedMutation := httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		Timeout:  5 * time.Second,
		Mutation: true,
	}
	jsonProtected := protectedMutation
	jsonProtected.BodyMax = jsonBodyMax
	jsonProtected.Timeout = 120 * time.Second

	// The SPA shell loads with the token OR — when a console password is
	// configured (M933) — with no credential at all, so a token-less visitor
	// gets the password login screen instead of a bare 401. The shell carries
	// no data; every data route stays protected by route policy.
	router.Handle("/", publicRead, s.shellAuth(s.handleSPA))
	// Auth surface (M817/M933): probe + login/logout are token-FREE — they are
	// how a token-less browser gets in. authmeta leaks only "is there a
	// password gate" (no data); login is bounded by the failed-attempt lockout.
	router.Handle("/api/authmeta", publicRead, s.handleAuthMeta)
	loginPublic := publicMutation
	loginPublic.BodyMax = loginBodyLimit
	router.Handle("/api/login", loginPublic, s.handleLogin)
	router.Handle("/api/logout", publicMutation, s.handleLogout)
	// The hashed bundle and favicon are PUBLIC: the browser loads them as
	// subresources of index.html and cannot attach the ?token= (only fetch /
	// EventSource, which the SPA controls, can). They carry no secrets (compiled
	// UI code). The data surfaces — /events and every /api/* — stay token-gated,
	// so an unauthenticated visitor gets the shell but no data.
	router.Handle("/assets/", publicRead, s.handleAssets())
	router.Handle("/favicon.ico", publicRead, handleFavicon)
	router.Handle("/events", protectedRead, s.handleEvents)
	// The console's route table lives in the operation catalogue
	// (kernel/contract/opapi/http.go): every route below is DERIVED from it —
	// the path, the operation and the forwarded argument allowlist are read
	// from one table, not from four hand-written maps kept in this package.
	// The streamed operations are the one kind this loop does not bind: their
	// frames are the payload, so each has a handler of its own — they are
	// registered one by one below.
	for _, route := range opapi.ConsoleRoutes() {
		switch route.Kind {
		case opapi.HTTPRouteRead:
			router.Handle(route.Path, proxyRead, s.proxy(route.Op))
		case opapi.HTTPRouteReadArgs:
			router.Handle(route.Path, proxyRead, s.readArgsProxy(route.Op, route.Args))
		case opapi.HTTPRouteWrite:
			router.Handle(route.Path, protectedMutation, s.writeProxy(route.Op, route.Args))
		case opapi.HTTPRouteJSON:
			router.Handle(route.Path, jsonProtected, s.jsonProxy(route.Op, route.Args))
		}
	}
	router.Handle("/api/rollback/checkpoints", protectedRead, s.handleRollbackCheckpoints)
	router.Handle("/api/rollback/apply", jsonProtected, s.handleRollbackApply)
	streamMutation := jsonProtected
	streamMutation.Timeout = planRunTimeout
	// The streamed operations bind a bespoke handler each — the frames ARE the
	// payload — but their path and their forwarded keys come from the same
	// catalogue as every other console route.
	router.Handle(consolePath(opapi.Plan), streamMutation, s.planRunProxy())
	router.Handle(consolePath(opapi.Run), streamMutation, s.runStreamProxy())
	// CLI Toolbox install (M956): runs the host package manager and streams a
	// per-tool progress event then a final summary as SSE.
	router.Handle(consolePath(opapi.ToolboxInstall), streamMutation, s.toolInstallProxy())
	// Marketplace install/uninstall stream per-item progress (skill/mcp/tool) as SSE.
	router.Handle(consolePath(opapi.MarketInstall), streamMutation, s.marketStreamProxy(opapi.MarketInstall))
	router.Handle(consolePath(opapi.MarketUninstall), streamMutation, s.marketStreamProxy(opapi.MarketUninstall))
	// SSE token endpoint (VULN query-string-token fix): returns the ephemeral
	// SSE-only token the SPA embeds in the EventSource URL (?st=...) instead of
	// reusing the main console token. Requires valid WebUI data authorization.
	sseTokenRead := protectedRead
	sseTokenRead.Method = "GET,OPTIONS"
	router.Handle("/api/sse-token", sseTokenRead, s.handleSSEToken)
	// Provider-free speech readiness probe. Unlike the STT/TTS action routes,
	// this never uploads dummy audio or synthesizes throwaway speech.
	router.Handle("/api/voice/status", protectedRead, s.handleVoiceStatus)
	transcribeProtected := protectedMutation
	transcribeProtected.Timeout = 0
	transcribeProtected.BodyMax = audioMaxBytes
	router.Handle("/api/transcribe", transcribeProtected, s.handleTranscribe)
	// Text-to-speech for the console Voice Mode (M998): POST {"text":…} and the
	// server streams synthesized audio from the configured TTS backend.
	ttsProtected := protectedMutation
	ttsProtected.Timeout = 0
	ttsProtected.BodyMax = ttsTextMaxBytes
	router.Handle("/api/tts", ttsProtected, s.handleTTS)
	// Binary artifact serving (M822): streams the raw bytes for a content ref with
	// a sanitized Content-Type, so an <img src> / download link can render stored
	// images and files. Proxies opapi.ArtifactGet (which re-verifies the bytes).
	artifactRead := protectedRead
	artifactRead.Timeout = 15 * time.Second
	router.Handle("/api/artifact/raw", artifactRead, s.handleArtifactRaw)
	// File Manager routes (M1017): live filesystem under the configured
	// `AGEZT_FILE_ROOT` (default `~/agezt/workspace`). Every endpoint here
	// path-traversal-guards before touching the OS — see files_route.go for
	// the chokepoint.
	router.Handle("/api/files/tree", protectedRead, s.handleFileTree)
	router.Handle("/api/files/raw", protectedRead, s.handleFileRaw)
	filesMutation := protectedMutation
	filesMutation.Timeout = 0
	filesMutation.BodyMax = defaultFileCap
	router.Handle("/api/files/mkdir", filesMutation, s.handleFileMkdir)
	router.Handle("/api/files/rename", filesMutation, s.handleFileRename)
	router.Handle("/api/files/delete", filesMutation, s.handleFileDelete)
	// Workflow webhooks (M809): the ONE deliberately console-token-free
	// path. Authentication is the per-workflow secret, verified by the
	// control plane (constant-time); all this handler can ever do is ask
	// "fire workflow <name>" — no reads, no other writes, uniform refusals.
	webhookMutation := publicMutation
	webhookMutation.BodyMax = webhookBodyCap
	webhookMutation.Timeout = 130 * time.Second
	router.Handle("/hooks/", webhookMutation, s.handleWorkflowHook)
	// Channel OAuth redirect target (Phase 4). Public (no console token): the
	// provider redirects the operator's browser here with ?code&state. Security
	// rests on the unguessable state minted by /api/channel/oauth/start.
	oauthCallback := publicRead
	oauthCallback.Mutation = true
	oauthCallback.Timeout = 30 * time.Second
	router.Handle("/oauth/callback", oauthCallback, s.handleOAuthCallback)
	return router
}

func (s *Server) Handler() http.Handler {
	return s.secure(s.routeRegistry().ServeHTTP)
}

// consolePath returns the path the console answers one operation at, read from
// the operation catalogue. An operation with no catalogue entry is a programmer
// error — the console would bind some other path, or none — so it panics at
// start-up instead of quietly serving a route nobody declared.
func consolePath(op string) string {
	route, ok := opapi.ConsoleRoute(op)
	if !ok {
		panic("webui: no console route for operation " + op)
	}
	return route.Path
}

// handleOAuthCallback receives the provider's authorization redirect
// (GET /oauth/callback?code=&state=), forwards it to the control plane to
// exchange the code and store the token, and renders a small self-closing page.
