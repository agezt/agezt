// SPDX-License-Identifier: MIT
//
// Server surface: New + SetTenantResolver/Authorizer + SetTranscriber +
// bind + Handler + audioMaxBytes. Split from openaiapi_server.go during
// Day 211 god-file refactor (#35).
// Public API unchanged.
package openaiapi

import (
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/bus"
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/httpserver"
)

func New(eng Engine, b *bus.Bus, token string) *Server {
	return &Server{
		eng:      eng,
		bus:      b,
		verifier: kernelauth.NewStaticVerifier(token),
	}
}

// SetTenantResolver enables tenant routing: requests carrying an X-Agezt-Tenant
// header are served by the resolved per-tenant Engine + bus.
func (s *Server) SetTenantResolver(r TenantResolver) { s.resolve = r }

// SetTenantAuthorizer enables per-tenant credentials: a request targeting a
// tenant (X-Agezt-Tenant header) may authorize with that tenant's own token
// instead of the daemon admin token. The admin token still authorizes any tenant.
func (s *Server) SetTenantAuthorizer(a TenantAuthorizer) { s.tenantAuth = a }

// SetTranscriber wires the speech-to-text backend for POST
// /v1/audio/transcriptions. Without it, that route reports "not configured".
func (s *Server) SetTranscriber(t Transcriber) { s.transcriber = t }

// bind resolves the Engine + bus for a request: the per-tenant pair when an
// X-Agezt-Tenant header is present and a resolver is configured, else the
// primary engine/bus.
func (s *Server) bind(r *http.Request) (Engine, *bus.Bus, error) {
	tenant := strings.TrimSpace(r.Header.Get("X-Agezt-Tenant"))
	if tenant == "" || s.resolve == nil {
		return s.eng, s.bus, nil
	}
	return s.resolve(tenant)
}

// Handler builds the mux; every route is token-authed.
func (s *Server) Handler() http.Handler {
	authenticator := httpserver.Authenticator{
		Verifier:        s.verifier,
		TenantAuthorize: httpserver.TenantAuthorizer(s.tenantAuth),
	}
	router := httpserver.NewRouter(authenticator, func(w http.ResponseWriter, _ *http.Request) {
		writeErr(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
	})
	jsonRoute := httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		BodyMax:  maxRequestBodyBytes,
		Mutation: true,
	}
	readRoute := httpserver.RouteOpts{Tier: kernelauth.TierUser, Method: http.MethodGet}
	router.Handle("/v1/chat/completions", jsonRoute, s.handleChat)
	router.Handle("/v1/responses", jsonRoute, s.handleResponses)
	router.Handle("/v1/models", readRoute, s.handleModels)
	// OpenAI's "retrieve model" — GET /v1/models/{id}. The list route above is an
	// EXACT match, so without this subtree handler a single-model GET (what the
	// official SDKs' models.retrieve(id) issues for capability probing) would 404.
	router.Handle("/v1/models/", readRoute, s.handleModelByID)
	router.Handle("/v1/audio/transcriptions", httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		BodyMax:  audioMaxBytes,
		Mutation: true,
	}, s.handleTranscription)
	return router
}

// audioMaxBytes bounds an uploaded audio body (OpenAI's own cap is 25 MiB).
const audioMaxBytes = 25 << 20
