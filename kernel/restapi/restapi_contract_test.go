// SPDX-License-Identifier: MIT

package restapi

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/contract/restapiapi"
	"github.com/agezt/agezt/kernel/httpserver"
)

// TestTheDaemonRegistersExactlyTheContractRoutes is the two-way ratchet
// between the contract (kernel/contract/restapiapi) and the daemon's route
// table: a route the contract names but the daemon does not serve is a 404 on
// a path every SDK was told to call, and a route the daemon serves but the
// contract does not name is a path nobody generated.
func TestTheDaemonRegistersExactlyTheContractRoutes(t *testing.T) {
	s := New(nil, nil, "token", "v1")
	router := s.Handler()
	registered, ok := router.(*httpserver.Router)
	if !ok {
		t.Fatalf("Handler returned %T, want *httpserver.Router", router)
	}
	got := registered.Routes()

	want := map[string]restapiapi.Route{}
	for _, r := range restapiapi.Routes() {
		want[r.Path] = r
	}
	if len(got) != len(want) {
		t.Fatalf("the daemon registers %d routes, the contract names %d", len(got), len(want))
	}
	for _, route := range got {
		contract, ok := want[route.Path]
		if !ok {
			t.Fatalf("the daemon serves %q, which the contract does not name", route.Path)
		}
		if route.Method != methodsOf(contract) {
			t.Fatalf("route %q serves %q, the contract says %q", contract.ID, route.Method, methodsOf(contract))
		}
		if route.Tier != tierOf(contract) {
			t.Fatalf("route %q serves tier %q, the contract says %q", contract.ID, route.Tier, tierOf(contract))
		}
		if route.Mutation != contract.Mutation {
			t.Fatalf("route %q: mutation=%v, the contract says %v", contract.ID, route.Mutation, contract.Mutation)
		}
		if contract.Mutation && route.BodyMax != bodyMaxOf(contract) {
			t.Fatalf("route %q caps the body at %d, the contract says %d", contract.ID, route.BodyMax, bodyMaxOf(contract))
		}
		if !contract.Mutation && route.BodyMax != 0 {
			t.Fatalf("route %q caps the body at %d but takes no body", contract.ID, route.BodyMax)
		}
	}
}

// methodsOf renders the contract's verbs the way the router records them.
func methodsOf(r restapiapi.Route) string {
	verbs := make([]string, 0, len(r.Methods)+1)
	for _, m := range r.Methods {
		verbs = append(verbs, m.Verb)
	}
	if r.Head {
		verbs = append(verbs, http.MethodHead)
	}
	sort.Strings(verbs)
	return strings.Join(verbs, ",")
}

// tierOf maps the contract's tier onto the kernel's.
func tierOf(r restapiapi.Route) kernelauth.Tier {
	switch r.Tier {
	case restapiapi.TierPublic:
		return kernelauth.TierPublic
	case restapiapi.TierAdmin:
		return kernelauth.TierAdmin
	default:
		return kernelauth.TierUser
	}
}

// bodyMaxOf is the body cap the contract asks for.
func bodyMaxOf(r restapiapi.Route) int64 {
	if r.BodyMax > 0 {
		return r.BodyMax
	}
	return maxRequestBodyBytes
}
