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
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/httpserver"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/webui"
)

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

//	AGEZT_WEB_ADDR  host:port to serve on; UNSET = ON at 127.0.0.1:8787.
//	                Set an explicit opt-out keyword ("off") to disable it —
//	                allow-by-default, like every other capability. (This line
//	                read "unset = off" until 2026-08-12, contradicting the
//	                switch sixteen lines below and the Config Center help; the
//	                security threat model had inherited the same error.)
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
	// bare Windows agezt.exe also gets a built-in first password so the operator
	// can browse to localhost and change it in Setup without env files.
	//
	// That built-in is MINTED PER INSTALL and persisted 0600 (SECRET-002). Until
	// 2026-08-13 it was `const defaultLoopbackWebPassword = "agezt"` — a
	// compile-time credential in a public repository, on a console that is ON by
	// default. In the default (non-strict) mode the password is a SUFFICIENT
	// credential, not a second factor (`authorized()` is token OR session), so
	// anything that could reach loopback — a second OS user, any local
	// non-operator process on a machine whose whole purpose is running
	// LLM-directed code — held every mutating route.
	//
	// The mint happens once, here, rather than inside the per-request password
	// function: the console password must be stable for the process, and the auth
	// gate must not touch the filesystem on every request.
	builtinPassword, mintedPassword := "", ""
	if isLoopback(ln.Addr().String()) && !webPasswordDefaultDisabled() {
		pw, minted, perr := ensureConsolePassword(baseDir)
		if perr != nil {
			// Fail CLOSED: no built-in password at all rather than a guessable
			// one. The tokened URL below still opens the console.
			fmt.Fprintf(stdout, "  console password : unavailable (%v) — use the tokened URL below\n", perr)
		} else {
			builtinPassword = pw
			if minted {
				mintedPassword = pw
			}
		}
	}
	webPassword := func() string { return effectiveWebPassword(builtinPassword, ln.Addr().String()) }
	wsrv.SetPasswordFn(webPassword)
	// A wildcard bind (0.0.0.0 / ::) reaches every interface, yet registers NO
	// allowed host — webAllowedHosts skips unspecified IPs — so the auto-raise
	// inside SetAllowedHosts never fires, even though hostAllowed accepts any IP
	// literal and the console is therefore LAN-reachable. Treat it as the
	// exposure it is.
	if h, _, splitErr := net.SplitHostPort(ln.Addr().String()); splitErr == nil {
		if ip := net.ParseIP(h); ip != nil && ip.IsUnspecified() {
			wsrv.SetPasswordStrict(true)
		}
	}
	// AGEZT_WEB_PASSWORD_STRICT is an explicit operator override in BOTH
	// directions — but only when actually set. An unset variable must not clear
	// the flag the exposure checks above just raised.
	//
	// AUTH-001 (2026-08-12): this used to assign unconditionally, one line after
	// SetAllowedHosts had auto-raised strict mode for a non-loopback host. The
	// env default is false, so an operator who set AGEZT_WEB_PASSWORD and bound
	// beyond loopback believed they had "token AND password" and silently got
	// "token OR password" — password alone then opened /api/run, /api/files/*
	// and /api/config/set.
	if raw, ok := os.LookupEnv(brand.EnvPrefix + "WEB_PASSWORD_STRICT"); ok {
		wsrv.SetPasswordStrict(strings.EqualFold(strings.TrimSpace(raw), "on"))
	}
	// Read the EFFECTIVE value back rather than re-deriving it from the env —
	// re-deriving is exactly what went wrong above. It feeds the boot banner and
	// the tunnel URL decision.
	passwordStrict := wsrv.PasswordStrict()
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
	// Show a FRESHLY minted built-in password once — on the boot that minted it,
	// and only when it is the password actually in force (an explicitly
	// configured AGEZT_WEB_PASSWORD outranks it). The operator has no other way
	// to learn it, and unlike the API listen tokens this banner already carries a
	// full credential by design: the tokened console URL a line above.
	// Subsequent boots point at the 0600 file rather than reprint the secret.
	if mintedPassword != "" && webPassword() == mintedPassword {
		desc += "  " + bannerColor("first-boot password: "+mintedPassword, "1;33") +
			bannerColor(" (shown once — stored in "+filepath.Join(baseDir, consolePasswordFile)+")", "1;32")
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

// consolePasswordFile holds the per-install built-in console password, beside
// the other 0600 credential files under the daemon's base directory.
const consolePasswordFile = "web-password"

// consolePasswordBytes is the entropy of a minted console password. 96 bits,
// rendered as 24 hex characters — shorter than a 256-bit listen token because a
// human pastes this one into a login form, and far beyond guessing for a
// credential that gates the whole control plane.
const consolePasswordBytes = 12

// ensureConsolePassword returns this install's built-in console password,
// minting and persisting one (0600) the first time it is asked. minted reports
// whether THIS call created it, which is what lets the caller print a fresh
// password in the boot banner exactly once.
//
// A read failure is returned rather than swallowed: minting a second password
// over a file we could not read would lock the operator out of the one already
// in force. An empty file is treated as absent (mint), which also self-heals a
// truncated write.
func ensureConsolePassword(baseDir string) (password string, minted bool, err error) {
	existing, err := kernelauth.ReadTokenFile(baseDir, consolePasswordFile)
	if err != nil {
		return "", false, err
	}
	if existing != "" {
		return existing, false, nil
	}
	raw := make([]byte, consolePasswordBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", false, fmt.Errorf("mint console password: %w", err)
	}
	pw := hex.EncodeToString(raw)
	if _, err := kernelauth.WriteTokenFile(baseDir, consolePasswordFile, pw); err != nil {
		return "", false, err
	}
	return pw, true, nil
}

// webPasswordDefaultDisabled reports the operator's explicit opt-out of the
// BUILT-IN password (AGEZT_WEB_PASSWORD_DEFAULT). Consulted both where the
// password is minted and where it is resolved, so the daemon never persists a
// credential it has been told not to use.
func webPasswordDefaultDisabled() bool {
	return envDisabled(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD_DEFAULT"))
}

// effectiveWebPassword resolves the console password for one bind. Precedence,
// highest first: an explicitly configured AGEZT_WEB_PASSWORD (re-read per gate
// decision, so Setup / Config Center apply live); the opt-out keyword on
// AGEZT_WEB_PASSWORD_DEFAULT, which turns the built-in off entirely; otherwise
// the per-install builtin, but only on a loopback bind — a console reachable
// beyond localhost gets no built-in password at all.
//
// builtin comes from ensureConsolePassword and is "" when the daemon could not
// mint one, which correctly degrades to "token only" rather than to a default.
func effectiveWebPassword(builtin, addr string) string {
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD")); v != "" {
		return v
	}
	if webPasswordDefaultDisabled() {
		return ""
	}
	if isLoopback(addr) {
		return builtin
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
