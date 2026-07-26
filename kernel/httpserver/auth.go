// SPDX-License-Identifier: MIT

package httpserver

import (
	"net/http"
	"strings"

	kernelauth "github.com/agezt/agezt/kernel/auth"
)

// TenantAuthorizer validates a tenant-scoped credential. It is consulted only
// for TierUser routes carrying a non-empty tenant header; TierAdmin routes can
// never be opened by a tenant credential.
type TenantAuthorizer func(tenant, presented string) bool

// UnauthorizedWriter preserves each surface's wire-specific error envelope.
type UnauthorizedWriter func(http.ResponseWriter, *http.Request)

// Authenticator adapts HTTP credentials into the transport-independent auth
// verifier. TenantHeader defaults to X-Agezt-Tenant.
type Authenticator struct {
	Verifier        kernelauth.Verifier
	TenantAuthorize TenantAuthorizer
	TenantHeader    string
}

// BearerToken extracts an exact, case-sensitive "Bearer " credential from the
// Authorization header. Query-string credentials are intentionally outside
// this shared API; the WebUI's constrained EventSource exception remains an
// explicit surface policy.
func BearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return token
	}
	return ""
}

// Authorized reports whether r satisfies required. Public routes do not need a
// configured verifier. Protected routes fail closed for a missing verifier,
// blank credential, unknown tier, or absent tenant scope.
func (a Authenticator) Authorized(r *http.Request, required kernelauth.Tier) bool {
	if !required.Valid() {
		return false
	}
	if required == kernelauth.TierPublic {
		return true
	}
	presented := BearerToken(r)
	if presented == "" {
		return false
	}
	if a.Verifier != nil && a.Verifier.Authorize(presented, required) {
		return true
	}
	if required != kernelauth.TierUser || a.TenantAuthorize == nil {
		return false
	}
	header := a.TenantHeader
	if header == "" {
		header = "X-Agezt-Tenant"
	}
	tenant := strings.TrimSpace(r.Header.Get(header))
	return tenant != "" && a.TenantAuthorize(tenant, presented)
}

// Middleware gates next at required and delegates unauthorized response
// formatting to reject. A nil reject callback uses a minimal plain-text 401.
func (a Authenticator) Middleware(
	required kernelauth.Tier,
	reject UnauthorizedWriter,
) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !a.Authorized(r, required) {
				if reject != nil {
					reject(w, r)
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				}
				return
			}
			next(w, r)
		}
	}
}
