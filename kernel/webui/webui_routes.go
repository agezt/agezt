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

	// The console's OWN endpoints (kernel/contract/opapi/native.go): the SPA
	// shell, the auth surface, the event feed, the file manager, the voice
	// surface, the rollback surface and the workflow hook. They are not
	// operation bindings, but they are part of the console's HTTP surface, so
	// their PATH, METHOD, TIER and MUTATION FLAG live in that one list, next to
	// the catalogue above. The console binds a handler to each by NAME — never
	// by path — so a path is written once in the whole tree.
	bindings := s.nativeBindings()
	bound := make(map[opapi.NativeRouteName]bool, len(bindings))
	for _, route := range opapi.ConsoleNativeRoutes() {
		binding, ok := bindings[route.Name]
		if !ok {
			// A route the contract names but the console does not answer is a
			// promise the console breaks at start-up, loudly, rather than a
			// 404 the browser discovers later.
			panic("webui: no handler for the console-native endpoint " + string(route.Name))
		}
		bound[route.Name] = true
		opts := httpserver.RouteOpts{
			Method:   route.Method,
			Timeout:  binding.timeout,
			BodyMax:  binding.bodyMax,
			Mutation: route.Mutation,
			Tier:     kernelauth.TierUser,
		}
		if route.Public {
			opts.Tier = kernelauth.TierPublic
		}
		router.Handle(route.Path, opts, binding.handler)
	}
	for name := range bindings {
		if !bound[name] {
			// A handler the console holds but the contract does not name is a
			// route nobody routes — the other half of the same promise.
			panic("webui: the console-native endpoint " + string(name) + " is not in the contract")
		}
	}

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
	return router
}

// nativeBinding is the console's half of a console-native endpoint: the
// handler that answers it, and the transport policy the console applies ON TOP
// of the contract's method, tier and mutation flag.
//
// The key is the contract's NAME, not a path. That is the whole point of the
// split: the path lives in the contract, so the console cannot write it — and
// so a handler cannot be bound to a path the contract does not name.
type nativeBinding struct {
	handler http.HandlerFunc
	timeout time.Duration
	bodyMax int64
}

// The console's own paths, read from the contract once at start-up. The
// authorization and hook handlers match on them per request, so they are
// resolved here rather than on every call.
var (
	consoleEventsPath       = opapi.ConsoleNativePath(opapi.NativeEvents)
	consoleWorkflowHookPath = opapi.ConsoleNativePath(opapi.NativeWorkflowHook)
)

// nativeBindings binds the console's own endpoints. Every key here must be a
// name the contract lists, and every name the contract lists must have a
// binding here — routeRegistry panics when either half is missing.
func (s *Server) nativeBindings() map[opapi.NativeRouteName]nativeBinding {
	return map[opapi.NativeRouteName]nativeBinding{
		opapi.NativeShell:         {handler: s.shellAuth(s.handleSPA)},
		opapi.NativeAuthMeta:      {handler: s.handleAuthMeta},
		opapi.NativeLogin:         {handler: s.handleLogin, bodyMax: loginBodyLimit},
		opapi.NativeLogout:        {handler: s.handleLogout},
		opapi.NativeAssets:        {handler: s.handleAssets()},
		opapi.NativeFavicon:       {handler: handleFavicon},
		opapi.NativeEvents:        {handler: s.handleEvents},
		opapi.NativeRollbackList:  {handler: s.handleRollbackCheckpoints},
		opapi.NativeRollbackApply: {handler: s.handleRollbackApply, timeout: 120 * time.Second, bodyMax: jsonBodyMax},
		opapi.NativeSSEToken:      {handler: s.handleSSEToken},
		opapi.NativeVoiceStatus:   {handler: s.handleVoiceStatus},
		opapi.NativeTranscribe:    {handler: s.handleTranscribe, bodyMax: audioMaxBytes},
		opapi.NativeTTS:           {handler: s.handleTTS, bodyMax: ttsTextMaxBytes},
		opapi.NativeArtifactRaw:   {handler: s.handleArtifactRaw, timeout: 15 * time.Second},
		opapi.NativeFilesTree:     {handler: s.handleFileTree},
		opapi.NativeFilesRaw:      {handler: s.handleFileRaw},
		opapi.NativeFilesMkdir:    {handler: s.handleFileMkdir, bodyMax: defaultFileCap},
		opapi.NativeFilesRename:   {handler: s.handleFileRename, bodyMax: defaultFileCap},
		opapi.NativeFilesDelete:   {handler: s.handleFileDelete, bodyMax: defaultFileCap},
		opapi.NativeWorkflowHook:  {handler: s.handleWorkflowHook, timeout: 130 * time.Second, bodyMax: webhookBodyCap},
		opapi.NativeOAuthCallback: {handler: s.handleOAuthCallback, timeout: 30 * time.Second},
	}
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
