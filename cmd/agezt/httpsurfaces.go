// SPDX-License-Identifier: MIT

// The daemon's HTTP product surfaces (Phase 2.6 file split out of main.go,
// same package): the Web UI resident + its password/token/host helpers, the
// tunnel supervisor, the OpenAI-compatible API, the native REST API, outbound
// webhooks, the shared listen-token writer, and the isLoopback exposure check
// their banners rely on.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/internal/brand"
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/openaiapi"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/kernel/restapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/tunnel"
	"github.com/agezt/agezt/kernel/update"
	"github.com/agezt/agezt/kernel/webhook"
	"github.com/agezt/agezt/kernel/webui"
)

// buildWebUI starts the Web UI resident when AGEZT_WEB_ADDR is set, on the
// daemon ctx (so `agt halt`/SIGTERM/`agt shutdown` stop it). It serves the SSE
// Live Monitor + read panels (SPEC-07) — token-authed and read-only (SPEC-06)
// — over the same bus and control plane the CLI uses, so the two views are
// guaranteed consistent. Returns a banner description (the tokenized URL), or
// "" when disabled.
type webUISurface struct {
	desc           string
	localURL       string
	token          string
	passwordOn     bool
	passwordStrict bool
	allowHost      func(string)
}

//	AGEZT_WEB_ADDR  host:port to serve on (e.g. 127.0.0.1:8787); unset = off.
//
// We never bind 0.0.0.0 implicitly: the operator supplies the host, and the
// banner warns if it isn't loopback (public exposure is their explicit choice,
// SPEC-06).
func buildWebUI(ctx context.Context, k *kernelruntime.Kernel, baseDir string, stdout io.Writer) webUISurface {
	// Default-ON (M817): the web console is the product surface, so a bare
	// `agezt` serves it without ceremony. AGEZT_WEB_ADDR overrides the bind
	// address; the explicit opt-OUT keywords disable it (mirrors the owner's
	// allow-by-default posture — you turn it off, you don't turn it on).
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_ADDR"))
	defaulted := false
	switch {
	case envDisabled(addr):
		return webUISurface{}
	case addr == "":
		addr = "127.0.0.1:8787"
		defaulted = true
	}
	// Fresh random token, minted like the control plane's (crypto/rand → hex).
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		fmt.Fprintf(stdout, "  web ui           : disabled (token mint failed: %v)\n", err)
		return webUISurface{}
	}
	token := hex.EncodeToString(tokBytes)

	// Reuse the same control-plane client `agt` builds — every read panel is a
	// proxied Cmd* call, so there is zero query duplication and full parity.
	client, err := controlplane.NewClient(baseDir)
	if err != nil {
		fmt.Fprintf(stdout, "  web ui           : disabled (control-plane client: %v)\n", err)
		return webUISurface{}
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil && defaulted {
		// The default port is taken (a second daemon, or another app on 8787).
		// Don't leave the console dark over a port clash — fall back to an
		// OS-assigned free port on loopback so a bare `agezt` ALWAYS gets a UI.
		fmt.Fprintf(stdout, "  web ui           : %s busy — using a free port instead\n", addr)
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		fmt.Fprintf(stdout, "  web ui           : disabled (listen %s: %v)\n", addr, err)
		return webUISurface{}
	}
	wsrv := webui.New(k.Bus(), client, token)
	wsrv.SetAllowedHosts(webAllowedHosts(ln.Addr().String())...)
	// Console password (M817 → M933): when AGEZT_WEB_PASSWORD is set, a token-less
	// visit shows the login screen and the password opens the console (alternative
	// door); the tokened banner URL keeps working alone. Wired as a LIVE source —
	// re-read from the env per gate decision — so setting the password from Setup /
	// Config Center applies without a restart. For the default loopback console, a
	// bare Windows agezt.exe also gets a built-in first password ("agezt") so the
	// operator can browse to localhost and change it in Setup without env files.
	webPassword := func() string { return effectiveWebPassword(ln.Addr().String()) }
	wsrv.SetPasswordFn(webPassword)
	passwordStrict := strings.EqualFold(os.Getenv(brand.EnvPrefix+"WEB_PASSWORD_STRICT"), "on")
	wsrv.SetPasswordStrict(passwordStrict)
	passwordOn := webPassword() != ""
	// Wire speech-to-text for the chat mic button (M689) and the console Voice
	// mode. Prefer the runtime voice adapter so native providers (ElevenLabs /
	// Deepgram), not just OpenAI-compatible endpoints, drive browser transcription;
	// fall back to the standalone AGEZT_STT_API_* client. Guard the concrete
	// pointer in the fallback so a nil never becomes a non-nil interface.
	if v := k.Voice(); v != nil && v.HasSTT() {
		wsrv.SetTranscriber(voiceTranscriberShim{v})
		fmt.Fprintf(stdout, "  voice input      : enabled (chat mic → speech-to-text)\n")
	} else if t := sttTranscriberFromEnv(); t != nil {
		wsrv.SetTranscriber(t)
		fmt.Fprintf(stdout, "  voice input      : enabled (chat mic → speech-to-text)\n")
	}
	// Wire text-to-speech for the console Voice Mode (M998) when TTS is configured.
	// Reuse the kernel's runtime voice adapter (built from AGEZT_TTS_* at boot) — it
	// already implements webui.Synthesizer (Speak). Guard HasTTS so an STT-only
	// daemon doesn't advertise a /api/tts that 502s; the browser falls back to its
	// built-in voice when this stays unwired.
	if v := k.Voice(); v != nil && v.HasTTS() {
		wsrv.SetSynthesizer(v)
		fmt.Fprintf(stdout, "  voice output     : enabled (voice mode → text-to-speech)\n")
	}
	httpserver.Start(ctx, ln, wsrv.Handler(), func(err error) {
		fmt.Fprintf(stdout, "web ui server error: %v\n", err)
	})

	localURL := "http://" + ln.Addr().String()
	consoleURL := localURL + "/?token=" + token
	plainURL := localURL + "/"
	desc := bannerColor(consoleURL, "1;36")
	if passwordOn {
		desc += "  " + bannerColor("(password login enabled at "+plainURL+")", "1;32")
	}
	if !isLoopback(addr) {
		desc += "  " + bannerColor("[WARNING: not loopback — reachable beyond localhost]", "1;33")
	}
	if shouldOpenWebUI() {
		if err := openBrowser(consoleURL); err != nil {
			desc += "  " + bannerColor("(browser auto-open failed: "+err.Error()+")", "1;33")
		} else {
			desc += "  " + bannerColor("(opened in browser)", "1;32")
		}
	}
	return webUISurface{
		desc:           desc,
		localURL:       localURL,
		token:          token,
		passwordOn:     passwordOn,
		passwordStrict: passwordStrict,
		allowHost:      func(host string) { wsrv.SetAllowedHosts(host) },
	}
}

func envDisabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "disabled", "none", "no", "0", "false":
		return true
	default:
		return false
	}
}

func bannerColor(s, code string) string {
	if !bannerColorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bannerColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.HasSuffix(os.Args[0], ".test") {
		return false
	}
	return true
}

const defaultLoopbackWebPassword = "agezt"

func effectiveWebPassword(addr string) string {
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD")); v != "" {
		return v
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD_DEFAULT"))) {
	case "off", "disabled", "none", "no", "0", "false":
		return ""
	}
	if isLoopback(addr) {
		return defaultLoopbackWebPassword
	}
	return ""
}

func shouldOpenWebUI() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_OPEN"))) {
	case "off", "disabled", "none", "no", "0", "false":
		return false
	}
	// `go test` must not launch a desktop browser.
	return !strings.HasSuffix(os.Args[0], ".test")
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch stdruntime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func webAllowedHosts(bindAddr string) []string {
	var hosts []string
	if h, _, err := net.SplitHostPort(bindAddr); err == nil {
		if ip := net.ParseIP(h); ip == nil || !ip.IsUnspecified() {
			hosts = append(hosts, h)
		}
	}
	for _, h := range strings.Split(os.Getenv(brand.EnvPrefix+"WEB_ALLOWED_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// buildTunnel starts a tunnel to a local HTTP service when AGEZT_TUNNEL
// (cloudflare/cloudflared|ngrok|tailscale|tailscale-funnel) or AGEZT_TUNNEL_CMD
// (a custom command) is set. It targets AGEZT_TUNNEL_TARGET, else the live Web
// UI listener, else the REST addr. The supervised binary's remote URL is printed
// to the daemon log once it connects. Returns "" (disabled) when no tunnel is
// configured.
//
//	AGEZT_TUNNEL         provider preset: cloudflare/cloudflared | ngrok | tailscale | tailscale-funnel
//	AGEZT_TUNNEL_CMD     explicit command (whitespace-split), overrides the preset
//	AGEZT_TUNNEL_TARGET  local URL to expose (default: the Web UI, else REST, addr)
func buildTunnel(ctx context.Context, stdout io.Writer, web webUISurface) string {
	provider := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL"))
	cmdStr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL_CMD"))
	if provider == "" && cmdStr == "" {
		return ""
	}

	target := tunnelTargetFromEnv(web)
	targetsWebUI := target != "" && sameURLTarget(target, web.localURL)

	cfg := tunnel.Config{
		Provider:  provider,
		TargetURL: target,
		OnURL: func(u string) {
			if targetsWebUI && web.allowHost != nil {
				if h := publicURLHost(u); h != "" {
					web.allowHost(h)
				}
			}
			public := tunnelPublicURL(u, web, targetsWebUI)
			fmt.Fprintf(stdout, "  tunnel URL       : %s  [the service is now reachable through the tunnel]\n", public)
			if targetsWebUI && web.passwordOn && web.passwordStrict {
				fmt.Fprintf(stdout, "  tunnel auth      : Web UI password strict mode is enabled; public host was allowlisted and token+password are required\n")
			} else if targetsWebUI && web.passwordOn {
				fmt.Fprintf(stdout, "  tunnel auth      : Web UI password is enabled; public URL opens password login and host was allowlisted automatically\n")
			} else if targetsWebUI {
				fmt.Fprintf(stdout, "  tunnel auth      : WARNING Web UI password is not set; access is token-only\n")
			}
		},
	}
	if cmdStr != "" {
		cfg.Command = strings.Fields(cmdStr)
	}

	tun, err := tunnel.New(cfg)
	if err != nil {
		fmt.Fprintf(stdout, "  tunnel           : disabled (%v)\n", err)
		return ""
	}
	go tun.Start(ctx)

	what := "custom command"
	if cmdStr == "" {
		what = provider
	}
	desc := fmt.Sprintf("%s → exposing %s (public URL prints here once connected)", what, target)
	if target == "" {
		desc = fmt.Sprintf("%s (custom command; no local target derived)", what)
	}
	return desc
}

func tunnelTargetFromEnv(web webUISurface) string {
	if target := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL_TARGET")); target != "" {
		return target
	}
	if web.localURL != "" {
		return web.localURL
	}
	if webAddr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_ADDR")); webAddr != "" && !envDisabled(webAddr) {
		return addrToURL(webAddr)
	}
	if rest := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REST_ADDR")); rest != "" && !envDisabled(rest) {
		return addrToURL(rest)
	}
	return ""
}

func sameURLTarget(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), strings.TrimRight(strings.TrimSpace(b), "/"))
}

func publicURLHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func tunnelPublicURL(raw string, web webUISurface, targetsWebUI bool) string {
	if targetsWebUI && web.token != "" && (!web.passwordOn || web.passwordStrict) {
		return urlWithToken(raw, web.token)
	}
	return raw
}

func urlWithToken(raw, token string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if u.Path == "" {
		u.Path = "/"
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

// addrToURL turns a listen addr (host:port, or :port) into a loopback http URL.
func addrToURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	return "http://" + addr
}

// writeAPIListenToken persists the freshly-minted HTTP listen token to a 0600
// file under the daemon's base directory and returns a short prefix suitable
// for surfacing in the boot banner without leaking the full secret. Banner
// leak audit (VULN banner-token-leak): the FULL token must NEVER appear on
// stdout/stderr or anywhere a log-shipper / `journalctl` / `agt status` will
// scrape it. Callers are expected to embed `prefix` in the banner and point
// operators at the file path for the live secret. On a write failure we
// return the empty prefix AND the error so the caller can fail closed rather
// than print nothing and silently leave the operator without the secret.
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
