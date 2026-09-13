// SPDX-License-Identifier: MIT
//
// cmd/agezt WebUI surface: webUISurface struct + buildWebUI (the
// constructor that wires up the static-file server + the API gateway
// + the SPA handler + the allowed-hosts callback).
// The password helpers live in httpsurfaces_pwd.go; the browser / runtime
// helpers live in httpsurfaces_browser.go.
// Extracted from httpsurfaces.go during the Day-208 god-file split.
// Public API unchanged.
package main

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

	"github.com/agezt/agezt/internal/brand"
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
