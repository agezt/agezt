// SPDX-License-Identifier: MIT

package httpserver

import (
	"net/http"
	"strings"
	"sync"
	"time"

	kernelauth "github.com/agezt/agezt/kernel/auth"
)

// RouteOpts declares the shared transport policy for a route. Method defaults
// to "*" while a surface is being migrated. A zero BodyMax leaves the body
// uncapped by this layer; handlers with specialized streaming limits may still
// apply their own cap. Timeout is inspectable metadata: handlers retain control
// of their wire-specific timeout/cancellation response.
type RouteOpts struct {
	Tier         kernelauth.Tier
	Method       string
	BodyMax      int64
	Timeout      time.Duration
	Mutation     bool
	Unauthorized UnauthorizedWriter
}

// Route is an inspectable snapshot of registered transport policy.
type Route struct {
	Method string
	Path   string
	// Pattern is retained as a compatibility alias for Path.
	// Deprecated: use Path.
	Pattern  string
	Tier     kernelauth.Tier
	BodyMax  int64
	Timeout  time.Duration
	Mutation bool
}

// Router wraps ServeMux with consistent authorization and body-limit policy.
// Registration is expected during startup. Routes is safe to inspect after
// registration while the server is running.
type Router struct {
	mux           *http.ServeMux
	authenticator Authenticator
	reject        UnauthorizedWriter

	mu     sync.RWMutex
	routes []Route
}

// NewRouter creates an empty route registry. reject is the surface-default 401
// writer and can be overridden per route.
func NewRouter(authenticator Authenticator, reject UnauthorizedWriter) *Router {
	return &Router{
		mux:           http.NewServeMux(),
		authenticator: authenticator,
		reject:        reject,
	}
}

// Handle registers pattern with its transport policy. Invalid tiers and
// negative limits panic at startup rather than silently weakening a route.
func (rt *Router) Handle(pattern string, opts RouteOpts, handler http.HandlerFunc) {
	if rt == nil {
		panic("httpserver: nil router")
	}
	if !opts.Tier.Valid() {
		panic("httpserver: invalid route tier")
	}
	if opts.BodyMax < 0 {
		panic("httpserver: body limit cannot be negative")
	}
	if opts.Timeout < 0 {
		panic("httpserver: route timeout cannot be negative")
	}
	methods := strings.Split(opts.Method, ",")
	if strings.TrimSpace(opts.Method) == "" {
		methods = []string{"*"}
	}
	normalizedMethods := make([]string, 0, len(methods))
	seenMethods := make(map[string]struct{}, len(methods))
	for _, candidate := range methods {
		method := strings.ToUpper(strings.TrimSpace(candidate))
		if method == "" || (method == "*" && len(methods) != 1) {
			panic("httpserver: invalid route method")
		}
		if method != "*" {
			if _, err := http.NewRequest(method, "/", nil); err != nil {
				panic("httpserver: invalid route method")
			}
		}
		if _, exists := seenMethods[method]; exists {
			continue
		}
		seenMethods[method] = struct{}{}
		normalizedMethods = append(normalizedMethods, method)
	}
	method := strings.Join(normalizedMethods, ",")
	if handler == nil {
		panic("httpserver: nil route handler")
	}

	wrapped := handler
	if method != "*" {
		// The declared method policy is enforced, not merely recorded: a
		// route registered Method: POST, Mutation: true must refuse GET — a
		// navigation/CSRF-class request otherwise reaches state-changing
		// handlers — and Routes() must report the policy the server actually
		// applies. Sits inside the auth wrapper so unauthenticated requests
		// keep their 401 regardless of method.
		wrapped = methodLimit(strings.Split(method, ","))(wrapped)
	}
	if opts.BodyMax > 0 {
		wrapped = BodyLimit(opts.BodyMax)(wrapped)
	}
	if opts.Tier != kernelauth.TierPublic {
		reject := opts.Unauthorized
		if reject == nil {
			reject = rt.reject
		}
		wrapped = rt.authenticator.Middleware(opts.Tier, reject)(wrapped)
	}
	rt.mux.HandleFunc(pattern, wrapped)

	rt.mu.Lock()
	rt.routes = append(rt.routes, Route{
		Method:   method,
		Path:     pattern,
		Pattern:  pattern,
		Tier:     opts.Tier,
		BodyMax:  opts.BodyMax,
		Timeout:  opts.Timeout,
		Mutation: opts.Mutation,
	})
	rt.mu.Unlock()
}

// Routes returns a copy of the registered route-policy metadata.
func (rt *Router) Routes() []Route {
	if rt == nil {
		return nil
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return append([]Route(nil), rt.routes...)
}

// ServeHTTP implements http.Handler.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.mux.ServeHTTP(w, r)
}

// methodLimit enforces a route's declared method allowlist ("POST" or
// "GET,OPTIONS"): any other method is refused with 405 and an Allow header.
func methodLimit(allowed []string) func(http.HandlerFunc) http.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, m := range allowed {
		set[m] = struct{}{}
	}
	allow := strings.Join(allowed, ", ")
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if _, ok := set[r.Method]; !ok {
				w.Header().Set("Allow", allow)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			next(w, r)
		}
	}
}
