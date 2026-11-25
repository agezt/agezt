package openaiapi

// Provenance: SPDX-License-Identifier: MIT Server surface: New +
//             SetTenantResolver/Authorizer + SetTranscriber + bind + Handler +
//             audioMaxBytes. Split from openaiapi_server.go during Day 211 god-file
//             refactor (#35). Public API unchanged.

import (
	"net/http"
	"strings"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/bus"
	oaiapi "github.com/agezt/agezt/kernel/contract/openaiapi"
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
//
// The surface has one home (kernel/contract/openaiapi): every route names its
// path, its method and the body it takes. The server registers FROM that
// table, so a route in the contract without a handler — or a handler without a
// route — stops the daemon at boot instead of answering a caller the contract
// promised.
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
	audioRoute := httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		BodyMax:  audioMaxBytes,
		Mutation: true,
	}
	handlers := map[string]http.HandlerFunc{
		oaiapi.RouteChatCompletions:     s.handleChat,
		oaiapi.RouteResponses:           s.handleResponses,
		oaiapi.RouteAudioTranscriptions: s.handleTranscription,
		oaiapi.RouteModelsList:          s.handleModels,
		oaiapi.RouteModelsRetrieve:      s.handleModelByID,
	}
	policies := map[string]httpserver.RouteOpts{
		oaiapi.RouteChatCompletions:     jsonRoute,
		oaiapi.RouteResponses:           jsonRoute,
		oaiapi.RouteAudioTranscriptions: audioRoute,
		oaiapi.RouteModelsList:          readRoute,
		oaiapi.RouteModelsRetrieve:      readRoute,
	}
	for _, r := range oaiapi.Routes() {
		h, ok := handlers[r.ID]
		if !ok {
			panic("openaiapi: contract route has no handler: " + r.ID)
		}
		delete(handlers, r.ID)
		opts, ok := policies[r.ID]
		if !ok {
			panic("openaiapi: contract route has no transport policy: " + r.ID)
		}
		delete(policies, r.ID)
		if opts.Method != r.Method {
			panic("openaiapi: contract route " + r.ID + " is a " + r.Method +
				", the transport policy says " + opts.Method)
		}
		router.Handle(r.Path, opts, h)
	}
	for id := range handlers {
		panic("openaiapi: handler has no contract route: " + id)
	}
	for id := range policies {
		panic("openaiapi: transport policy has no contract route: " + id)
	}
	return router
}

// audioMaxBytes bounds an uploaded audio body (OpenAI's own cap is 25 MiB).
const audioMaxBytes = 25 << 20
