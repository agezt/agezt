// SPDX-License-Identifier: MIT

// Web UI server: Caller/Transcriber/Synthesizer interfaces + Server struct + SetTranscriber/SetSynthesizer/SetAllowedHosts + New.
// Code extracted from webui.go during the Day-51 god-file split. Public API unchanged.
package webui


import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net"
	"strings"
	"sync"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)



// Caller is the API the dashboard proxies to — satisfied by
// *controlplane.Client. An interface keeps webui testable without a live
// daemon (a fake Caller + an in-memory bus is enough).
//
// Call handles single request→response commands (every read panel, the
// query-arg writes). Stream handles a command that emits a sequence of
// RespEvent frames before its terminal result — currently only CmdPlan, used
// by Flow Studio's "Run". The streamed events are discarded here (the browser
// already sees them live on the SSE /events firehose); Stream is driven to its
// terminal result only so the control-plane connection stays open for the
// plan's whole duration — dropping it early would cancel the run's context.
type Caller interface {
	Call(ctx context.Context, cmd string, args map[string]any) (map[string]any, error)
	Stream(ctx context.Context, cmd string, args map[string]any, onEvent func(*event.Event)) (map[string]any, error)
}

// Transcriber turns uploaded audio into text — the speech-to-text backend behind
// the chat mic button. Satisfied by *stt.Client (the same one the OpenAI-API
// surface uses). Optional: when nil, /api/transcribe reports "not configured".
type Transcriber interface {
	Transcribe(ctx context.Context, filename string, audio []byte) (string, error)
}

// Synthesizer turns text into spoken audio — the text-to-speech backend behind
// the console Voice Mode's /api/tts route. Satisfied by *voice.Adapter (the same
// OpenAI-compatible TTS half agents use to speak replies). Optional: when nil,
// /api/tts reports "not configured" so the browser falls back to its built-in
// SpeechSynthesis voice.
type Synthesizer interface {
	Speak(ctx context.Context, text string) (audio []byte, mime string, err error)
}

// Server is the Web UI HTTP surface.
type Server struct {
	bus         *bus.Bus
	client      Caller
	token       string // main console / API bearer token
	sseToken    string // ephemeral SSE-only token for /events EventSource URL
	tokenAuth   kernelauth.Verifier
	sseAuth     kernelauth.Verifier
	dist        fs.FS       // the built SPA bundle (embed dist/, sub-rooted)
	transcriber Transcriber // optional STT backend for /api/transcribe (nil = not configured)
	synthesizer Synthesizer // optional TTS backend for /api/tts (nil = not configured)
	// passwordFn is the LIVE password source (M933), so a Setup/Config-Center
	// edit applies without a restart.
	passwordFn func() string
	// passwordStrict restores M817 compose (token AND session) instead of the
	// M933 alternative-door default (token OR session).
	passwordStrict bool
	sessions       *sessionStore
	allowedHosts   map[string]bool
	hostPolicyMu   sync.RWMutex
	// allowQueryTokensForData preserves legacy package tests that still pass
	// ?token= to /api/*; production data routes use Bearer/session, except
	// /events where EventSource cannot set Authorization.
	allowQueryTokensForData bool

	// hookRL throttles the token-free POST /hooks/ webhook (VULN-007). The path is
	// authenticated only by a per-workflow secret; without a frequency cap, anyone
	// who learns one secret could loop it and launch unbounded governed (paid) runs
	// up to the soft daily budget. Buckets are keyed by "workflow|source-IP" so one
	// abusive source on one workflow is throttled without starving distinct legit
	// senders. Lazily initialised; idle buckets are evicted.
	hookRLMu   sync.Mutex
	hookRL     map[string]*hookBucket
	hookRLOnce sync.Once
}

// SetTranscriber wires the speech-to-text backend for POST /api/transcribe
// (the chat mic button). Without it, that route reports "not configured" so the
// UI can degrade gracefully. Called once at startup, before Handler().
func (s *Server) SetTranscriber(t Transcriber) { s.transcriber = t }

// SetSynthesizer wires the text-to-speech backend for POST /api/tts (the console
// Voice Mode's spoken replies). Without it, that route reports "not configured"
// so the browser degrades to its built-in voice. Called once at startup, before
// Handler().
func (s *Server) SetSynthesizer(t Synthesizer) { s.synthesizer = t }

// SetAllowedHosts permits additional non-IP Host header values for deployments
// behind a domain or tunnel. Loopback names and IP literals are accepted by
// default; explicit hosts are for DNS names such as a reverse-proxy hostname.
//
// When ANY non-loopback host is registered (i.e. the console is reachable
// beyond localhost), password-strict mode auto-activates so that a guessed
// password alone is insufficient — the bearer token is also required (token
// AND session). The operator can override via SetPasswordStrict(false) if
// two-factor is not desired (VULN token-or-password-mode).
func (s *Server) SetAllowedHosts(hosts ...string) {
	if len(hosts) == 0 {
		return
	}
	s.hostPolicyMu.Lock()
	defer s.hostPolicyMu.Unlock()
	if s.allowedHosts == nil {
		s.allowedHosts = map[string]bool{}
	}
	for _, h := range hosts {
		if host := hostName(h); host != "" {
			s.allowedHosts[strings.ToLower(host)] = true
			// Any single explicit (non-loopback) host trips strict mode:
			// the console is reachable beyond localhost, so a guessed
			// password alone must not open data routes (VULN token-or-
			// password-mode). The operator can override via
			// SetPasswordStrict(false).
			if !strings.EqualFold(host, "localhost") {
				if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
					s.passwordStrict = true
				}
			}
		}
	}
}

// New builds a Server. token gates every request; bus drives the live feed;
// client proxies read commands. The embedded SPA bundle is sub-rooted so the
// FileServer serves dist/ contents at the URL root.
func New(b *bus.Bus, client Caller, token string) *Server {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist is embedded at compile time; a failure here is a build defect, not
		// a runtime condition. Fall back to the raw FS so the server still starts.
		sub = distFS
	}
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	sseToken := hex.EncodeToString(buf)
	return &Server{
		bus:       b,
		client:    client,
		token:     token,
		sseToken:  sseToken,
		tokenAuth: kernelauth.NewStaticVerifier(token),
		sseAuth:   kernelauth.NewStaticVerifier(sseToken),
		dist:      sub,
		sessions:  newSessionStore(),
	}
}

// apiRoutes maps each GET /api path to the read-only control-plane command it
// proxies. Read-only by construction: there is no path here that mutates.