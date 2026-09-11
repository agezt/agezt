// SPDX-License-Identifier: MIT

// REST API core: types (Caller/Engine/Server/Config) + New + SetTenantResolver/Authorizer/Readiness/Metrics/UpdateService + bind.
// Code extracted from restapi.go during the Day-66 god-file split. Public API unchanged.
package restapi


import (
	"context"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/update"
	"net/http"
	"strings"
)



// maxRequestBodyBytes caps an HTTP request body (M198). The API surfaces are
// network-exposed and token-authed, but a token holder (or a compromised/buggy
// client) must not be able to OOM the daemon with a giant JSON body. 16 MiB is
// far above any legitimate run/chat request.
const maxRequestBodyBytes = 16 << 20

// Engine is the slice of the kernel this server drives. It is satisfied
// structurally by the daemon's kernel adapter (the same one kernel/openaiapi
// uses, plus EventsForCorrelation for run inspection).
type Engine interface {
	NewCorrelation() string
	SubjectForRun(corr string) string
	RunModel(ctx context.Context, corr, intent, model string, images []string, jsonMode bool) (string, error)
	DefaultModel() string
	ModelIDs() []string
	// EventsForCorrelation returns the journaled events of a run, in order.
	// Empty (not an error) when the correlation is unknown.
	EventsForCorrelation(corr string) ([]*event.Event, error)
}

// TenantResolver maps a tenant id to the Engine + bus that serve it. The daemon
// injects one (backed by the tenant registry) when multi-tenancy is enabled.
type TenantResolver func(tenant string) (Engine, *bus.Bus, error)

// TenantAuthorizer reports whether presented is the per-tenant credential of
// tenant id. The daemon injects one backed by the registry; it lets a scoped
// per-tenant token authorize requests against ONLY its own tenant, while the
// daemon admin token continues to authorize any tenant.
type TenantAuthorizer func(tenant, presented string) bool

// Server is the native REST surface.
type Server struct {
	eng      Engine
	bus      *bus.Bus
	verifier kernelauth.Verifier
	version  string

	// resolve, when set, maps the X-Agezt-Tenant request header to a per-tenant
	// Engine + bus. Nil (or an empty header) means the primary engine/bus —
	// the unchanged single-tenant path.
	resolve TenantResolver

	// tenantAuth, when set, validates a per-tenant token against the tenant named
	// in the X-Agezt-Tenant header. Nil means only the admin token authorizes.
	tenantAuth TenantAuthorizer

	// readiness, when set, reports whether the daemon can serve work right now
	// (e.g. not halted) for the unauthenticated /readyz probe. Nil → always ready
	// (the server answering at all proves liveness). Injected by the daemon so
	// this package needs no kernel halt-state coupling.
	readiness func() (ready bool, reason string)

	// metrics, when set, supplies the gauges exposed at /metrics in Prometheus
	// text format. Injected by the daemon (it has the kernel + governor); this
	// package only formats. Nil → /metrics reports no samples.
	metrics func() []Metric

	// board is the daemon's ONE shared message-board instance behind the
	// /api/v1/mailbox surface (M937), injected via SetMailbox. It must be the
	// same instance the `board` tool writes — see SetMailbox. Nil → the mailbox
	// endpoints answer 503.
	board *board.Store

	// boardNotify publishes board.posted for a mailbox write (the same closure
	// the `board` tool's OnPost uses), so an SDK send wakes standing orders
	// exactly like an agent's send. Nil-safe.
	boardNotify func(m board.Message, corr string)

	// updateSvc, when set, powers the /api/v1/update endpoints. Nil when update
	// is disabled; the handlers report that rather than dereferencing nil.
	updateSvc *update.Service
}

// Metric is one Prometheus sample exposed at /metrics. Name is the suffix after
// the `agezt_` prefix (e.g. "active_runs" → `agezt_active_runs`).
type Metric struct {
	Name  string
	Help  string
	Type  string // "gauge" or "counter"
	Value float64
}

// New builds a Server. token gates every request; bus drives streaming;
// version is reported by /health.
func New(eng Engine, b *bus.Bus, token, version string) *Server {
	return &Server{
		eng:      eng,
		bus:      b,
		verifier: kernelauth.NewStaticVerifier(token),
		version:  version,
	}
}

// SetTenantResolver enables tenant routing: requests carrying an X-Agezt-Tenant
// header are served by the resolved per-tenant Engine + bus.
func (s *Server) SetTenantResolver(r TenantResolver) { s.resolve = r }

// SetTenantAuthorizer enables per-tenant credentials: a request targeting a
// tenant (X-Agezt-Tenant header) may authorize with that tenant's own token
// instead of the daemon admin token. The admin token still authorizes any tenant.
func (s *Server) SetTenantAuthorizer(a TenantAuthorizer) { s.tenantAuth = a }

// SetReadiness injects the readiness probe behind the unauthenticated /readyz
// endpoint: it returns (false, reason) when the daemon can't serve work (e.g.
// halted). When unset, /readyz reports ready.
func (s *Server) SetReadiness(fn func() (ready bool, reason string)) { s.readiness = fn }

// SetMetrics injects the gauge source for /metrics (Prometheus text format).
func (s *Server) SetMetrics(fn func() []Metric) { s.metrics = fn }

// SetUpdateService wires the self-update engine. Nil when update is disabled;
// the update handlers report that rather than dereferencing nil.
func (s *Server) SetUpdateService(svc *update.Service) { s.updateSvc = svc }

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

// Handler builds the mux. The /api/v1/* routes are token-authed; the /healthz
// and /readyz probes are intentionally UNAUTHENTICATED so deployment tooling
// (systemd watchdog, container/k8s liveness+readiness probes, load balancers,
// uptime monitors) can check the daemon without a credential. They expose only
// liveness/readiness — never version, model, or any run data (that stays behind
// the authed /api/v1/health).