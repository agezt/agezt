// SPDX-License-Identifier: MIT

package httpserver

import (
	"net/http"
	"sync"

	kernelauth "github.com/agezt/agezt/kernel/auth"
)

// RouteOpts declares the shared transport policy for a route. A zero BodyMax
// leaves the body uncapped by this layer; handlers with specialized streaming
// limits may still apply their own cap.
type RouteOpts struct {
	Tier         kernelauth.Tier
	BodyMax      int64
	Unauthorized UnauthorizedWriter
}

// Route is an inspectable snapshot of registered transport policy.
type Route struct {
	Pattern string
	Tier    kernelauth.Tier
	BodyMax int64
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
	if handler == nil {
		panic("httpserver: nil route handler")
	}

	wrapped := handler
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
		Pattern: pattern,
		Tier:    opts.Tier,
		BodyMax: opts.BodyMax,
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
