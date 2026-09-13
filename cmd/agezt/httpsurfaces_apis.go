// SPDX-License-Identifier: MIT

package main

// Daemon API surfaces: buildOpenAIAPI + buildRESTAPI + buildWebhooks +
// writeAPIListenToken + restMetrics + isLoopback. Carved out of
// httpsurfaces.go during the Day 161 god-file split.
// Public API unchanged.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/internal/brand"
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/openaiapi"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/kernel/restapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/update"
	"github.com/agezt/agezt/kernel/webhook"
)
func writeAPIListenToken(baseDir, filename, token string) (prefix string, err error) {
	return kernelauth.WriteTokenFile(baseDir, filename, token)
}

// buildOpenAIAPI starts the OpenAI-compatible HTTP resident when AGEZT_API_ADDR
// is set, mirroring buildWebUI's lifecycle (daemon ctx, graceful shutdown,
// minted token, loopback warning). Returns the banner description or "".
//
// The minted bearer token is written to <baseDir>/openai.token with 0600 perms
// and only a short prefix is shown in the banner — the FULL token must NEVER
// appear on stdout/stderr where a log-shipper or `journalctl` would scrape it
// (VULN banner-token-leak fix).
func buildOpenAIAPI(ctx context.Context, k *kernelruntime.Kernel, reg *tenant.Registry, baseDir string, stdout io.Writer) string {
	addr := os.Getenv(brand.EnvPrefix + "API_ADDR")
	if addr == "" {
		return ""
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		fmt.Fprintf(stdout, "  openai api       : disabled (token mint failed: %v)\n", err)
		return ""
	}
	token := hex.EncodeToString(tokBytes)
	prefix, tokErr := writeAPIListenToken(baseDir, "openai.token", token)
	if tokErr != nil {
		fmt.Fprintf(stdout, "  openai api       : disabled (token persist failed: %v)\n", tokErr)
		return ""
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stdout, "  openai api       : disabled (listen %s: %v)\n", addr, err)
		return ""
	}
	api := openaiapi.New(kernelAPIEngine{k}, k.Bus(), token)
	if reg != nil {
		// Tenant routing: an X-Agezt-Tenant header serves the request from that
		// tenant's isolated kernel + bus (opened on demand).
		api.SetTenantResolver(func(id string) (openaiapi.Engine, *bus.Bus, error) {
			eng, b, err := tenantAPIEngine(reg, id)
			if err != nil {
				return nil, nil, err
			}
			return eng, b, nil
		})
		api.SetTenantAuthorizer(reg.Authorize)
	}
	// Speech-to-text upload (POST /v1/audio/transcriptions) — wired when an STT
	// endpoint is configured (a key, or a custom URL for a local whisper server).
	// Same source of truth as the Web UI mic button (M689).
	if t := sttTranscriberFromEnv(); t != nil {
		api.SetTranscriber(t)
	}
	httpserver.Start(ctx, ln, api.Handler(), func(err error) {
		fmt.Fprintf(stdout, "openai api server error: %v\n", err)
	})

	desc := "http://" + ln.Addr().String() + "/v1  (Authorization: Bearer " + prefix + "  — full token in " + filepath.Join(baseDir, "openai.token") + ")"
	if !isLoopback(addr) {
		desc += "  [WARNING: not loopback — reachable beyond localhost]"
	}
	return desc
}

// buildRESTAPI starts the native REST resident when AGEZT_REST_ADDR is set,
// mirroring buildOpenAIAPI's lifecycle (daemon ctx, graceful shutdown, minted
// token, loopback warning). Returns the banner description or "".
//
// The minted bearer token is written to <baseDir>/rest.token with 0600 perms
// and only a short prefix is shown in the banner — the FULL token must NEVER
// appear on stdout/stderr where a log-shipper or `journalctl` would scrape it
// (VULN banner-token-leak fix).
func buildRESTAPI(ctx context.Context, k *kernelruntime.Kernel, reg *tenant.Registry, baseDir string, draining *atomic.Bool, boardStore *board.Store, boardNotify func(board.Message, string), updateSvc *update.Service, stdout io.Writer) string {
	addr := os.Getenv(brand.EnvPrefix + "REST_ADDR")
	if addr == "" {
		return ""
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		fmt.Fprintf(stdout, "  rest api         : disabled (token mint failed: %v)\n", err)
		return ""
	}
	token := hex.EncodeToString(tokBytes)
	prefix, tokErr := writeAPIListenToken(baseDir, "rest.token", token)
	if tokErr != nil {
		fmt.Fprintf(stdout, "  rest api         : disabled (token persist failed: %v)\n", tokErr)
		return ""
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stdout, "  rest api         : disabled (listen %s: %v)\n", addr, err)
		return ""
	}
	rest := restapi.New(kernelAPIEngine{k}, k.Bus(), token, brand.Version)
	// Mailbox (M937): the shared message board for SDK apps — the same store
	// instance the `board` tool writes, so external sends wake standing orders
	// exactly like agent sends. Nil when the board failed to open.
	if boardStore != nil {
		rest.SetMailbox(boardStore, boardNotify)
	}
	// Self-update engine (M860): wired when AGEZT_UPDATE_ENDPOINT or
	// AGEZT_UPDATE_GITHUB_OWNER/REPO is set. Nil when not configured;
	// the update handlers report that.
	if updateSvc != nil {
		rest.SetUpdateService(updateSvc)
	}
	// Readiness probe (M134): /readyz reports not-ready while the daemon is
	// halted, so a load balancer / k8s readiness probe pulls it from rotation
	// without the process dying. Liveness (/healthz) stays up regardless.
	rest.SetReadiness(func() (bool, string) {
		if draining.Load() {
			return false, "draining"
		}
		if k.IsHalted() {
			return false, "halted"
		}
		return true, ""
	})
	// Prometheus /metrics (M135): expose the cheap in-memory operational gauges
	// (same data as status/budget/disk) so the daemon can be wired into Grafana /
	// alerting. All reads are O(1) or O(segments); no per-scrape journal fold.
	rest.SetMetrics(func() []restapi.Metric { return restMetrics(k) })
	if reg != nil {
		// Tenant routing: an X-Agezt-Tenant header serves the request from that
		// tenant's isolated kernel + bus (opened on demand).
		rest.SetTenantResolver(func(id string) (restapi.Engine, *bus.Bus, error) {
			eng, b, err := tenantAPIEngine(reg, id)
			if err != nil {
				return nil, nil, err
			}
			return eng, b, nil
		})
		rest.SetTenantAuthorizer(reg.Authorize)
	}
	httpserver.Start(ctx, ln, rest.Handler(), func(err error) {
		fmt.Fprintf(stdout, "rest api server error: %v\n", err)
	})

	desc := "http://" + ln.Addr().String() + "/api/v1  (Authorization: Bearer " + prefix + "  — full token in " + filepath.Join(baseDir, "rest.token") + ")"
	if !isLoopback(addr) {
		desc += "  [WARNING: not loopback — reachable beyond localhost]"
	}
	return desc
}

// buildWebhooks starts the outbound-webhook dispatcher when AGEZT_WEBHOOKS is
// set. It subscribes to the bus on the daemon ctx (so halt/shutdown stop it) and
// POSTs matching events to the configured sinks. Returns the banner description;
// "" only when the env var is unset (an empty/invalid spec returns a one-line
// reason so the operator sees the misconfiguration).
func buildWebhooks(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEBHOOKS"))
	if spec == "" {
		return ""
	}
	sinks, err := webhook.ParseSinks(spec)
	if err != nil {
		return "disabled (" + err.Error() + ")"
	}
	if len(sinks) == 0 {
		return ""
	}
	// Egress guard (M416, SPEC-06): outbound webhook deliveries are subject to the
	// same default-deny egress policy as the http/browser tools, so a configured
	// sink cannot reach loopback / RFC1918 / the cloud-metadata endpoint. Operators
	// who legitimately deliver to an internal sink opt the range back in.
	var guardOpts []netguard.Option
	egress := "guarded"
	if os.Getenv(brand.EnvPrefix+"WEBHOOK_ALLOW_LOOPBACK") == "1" {
		guardOpts = append(guardOpts, netguard.AllowLoopback())
		egress = "loopback-ok"
	}
	if os.Getenv(brand.EnvPrefix+"WEBHOOK_ALLOW_PRIVATE") == "1" {
		guardOpts = append(guardOpts, netguard.AllowPrivate())
		if egress == "loopback-ok" {
			egress = "loopback+private-ok"
		} else {
			egress = "private-ok"
		}
		fmt.Fprintln(stdout, "WARNING: AGEZT_WEBHOOK_ALLOW_PRIVATE=1 lets webhook sinks reach the private network.")
	}
	client := netguard.New(guardOpts...).HTTPClient(webhook.DefaultTimeout)
	webhook.NewDispatcher(k.Bus(), sinks, stdout, webhook.WithClient(client)).Start(ctx)
	return webhook.Describe(sinks) + " [egress=" + egress + "]"
}

// isLoopback reports whether the host portion of addr binds to loopback only.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false // empty host = all interfaces
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// restMetrics computes one snapshot of the Prometheus /metrics gauges (M135):
// the cheap in-memory operational data (same as status/budget/disk) so the
// daemon can be wired into Grafana / alerting. All reads are O(1) or
// O(segments); no per-scrape journal fold. Lifted out of buildRESTAPI's
// SetMetrics closure (Phase 2.6) — the wiring there is
// rest.SetMetrics(func() []restapi.Metric { return restMetrics(k) }).
func restMetrics(k *kernelruntime.Kernel) []restapi.Metric {
	boolf := func(b bool) float64 {
		if b {
			return 1
		}
		return 0
	}
	schedTotal, schedEnabled := 0, 0
	if st := k.Schedules(); st != nil {
		for _, e := range st.List() {
			schedTotal++
			if e.Enabled {
				schedEnabled++
			}
		}
	}
	headSeq, _ := k.Journal().Head()
	if headSeq < 0 {
		headSeq = 0
	}
	var spent, ceiling int64
	if gov, ok := k.Provider().(*governor.Governor); ok {
		snap := gov.Snapshot()
		spent, ceiling = snap.SpentMicrocents, snap.CeilingMicrocents
	}
	base := k.BaseDir()
	var journalBytes int64
	_ = filepath.Walk(filepath.Join(base, "journal"), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			journalBytes += info.Size()
		}
		return nil
	})
	var diskFree, diskTotal uint64
	if f, tot, err := pulse.DiskUsage(base); err == nil {
		diskFree, diskTotal = f, tot
	}
	diskRatio := 0.0
	if diskTotal > 0 {
		diskRatio = float64(diskFree) / float64(diskTotal)
	}
	pending := 0
	if ap := k.Approvals(); ap != nil {
		pending = ap.PendingCount()
	}
	return []restapi.Metric{
		{Name: "up", Help: "1 if the daemon is serving", Value: 1},
		{Name: "halted", Help: "1 if the daemon is halted", Value: boolf(k.IsHalted())},
		{Name: "uptime_seconds", Help: "seconds since the daemon started", Value: time.Since(k.StartTime()).Seconds()},
		{Name: "active_runs", Help: "runs currently in flight", Value: float64(k.ActiveRuns())},
		{Name: "journal_head_seq", Help: "latest journal sequence number", Value: float64(headSeq)},
		{Name: "memory_records", Help: "live memory records", Value: float64(k.Memory().Count())},
		{Name: "world_entities", Help: "live world-model entities", Value: float64(k.World().Count())},
		{Name: "active_skills", Help: "active skills", Value: float64(k.Forge().Count())},
		{Name: "schedules_total", Help: "scheduled intents", Value: float64(schedTotal)},
		{Name: "schedules_enabled", Help: "enabled scheduled intents", Value: float64(schedEnabled)},
		{Name: "pending_approvals", Help: "HITL approvals awaiting an operator", Value: float64(pending)},
		{Name: "spend_today_microcents", Help: "today's spend in microcents ($1=1e9)", Value: float64(spent)},
		{Name: "budget_ceiling_microcents", Help: "daily budget ceiling in microcents (0=unbounded)", Value: float64(ceiling)},
		{Name: "journal_bytes", Help: "journal size on disk in bytes", Value: float64(journalBytes)},
		{Name: "disk_free_bytes", Help: "free bytes on the journal filesystem", Value: float64(diskFree)},
		{Name: "disk_free_ratio", Help: "free fraction of the journal filesystem (0..1)", Value: diskRatio},
	}
}
