// SPDX-License-Identifier: MIT

// Package tunnel exposes a local Agezt HTTP service (the Web UI or REST API) to
// the public internet by supervising an operator-chosen tunnel binary —
// cloudflared or ngrok out of the box, or any custom command. It spawns the
// binary pointed at the local address, scans its output for the public URL it
// prints, surfaces that URL (startup log + `agt status`), restarts it with
// backoff if it drops, and tears the whole process tree down on shutdown.
//
// Why wrap an external binary rather than build a relay: a tunnel needs a public
// rendezvous server, which is exactly what cloudflared/ngrok already operate
// (battle-tested, free tiers). Agezt already supervises external processes
// (plugins, coding agents), so this fits the model — and keeps the daemon's one
// dependency promise (net/os/exec only, no SDK). The operator stays in authority:
// nothing is exposed unless they configure a tunnel, and they pick the provider.
package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Config constructs a Tunnel.
type Config struct {
	// Provider is a built-in preset: "cloudflare"/"cloudflared" or "ngrok".
	// "custom" requires Command. Ignored when Command is set.
	Provider string
	// Command is an explicit command + args to run (overrides Provider), for any
	// tunnel binary not covered by a preset. The operator embeds the target.
	Command []string
	// TargetURL is the local service to expose, e.g. http://127.0.0.1:8787.
	// Required for a preset; the presets insert it into the command.
	TargetURL string
	// OnURL, if set, is called each time a new public URL is discovered (e.g. to
	// log it). May be called from the supervisor goroutine.
	OnURL func(string)
}

// Tunnel supervises a tunnel subprocess and exposes the public URL it advertises.
type Tunnel struct {
	cmd   []string
	run   runFunc
	onURL func(string)

	mu  sync.RWMutex
	url string
}

// runFunc runs name+args until the process exits (or ctx is cancelled), calling
// onLine for every line the process writes to stdout or stderr. Injectable so
// tests drive the supervisor without a real subprocess.
type runFunc func(ctx context.Context, name string, args []string, onLine func(string)) error

// supervisor backoff bounds.
const (
	minBackoff    = 1 * time.Second
	maxBackoff    = 30 * time.Second
	healthyUptime = 30 * time.Second // a run lasting this long resets the backoff
)

// New builds a Tunnel from cfg. It resolves the command (explicit Command, or a
// provider preset around TargetURL) and errors if neither is usable.
func New(cfg Config) (*Tunnel, error) {
	cmd, err := buildCommand(cfg)
	if err != nil {
		return nil, err
	}
	return &Tunnel{cmd: cmd, run: execRun, onURL: cfg.OnURL}, nil
}

// URL returns the public URL the tunnel currently advertises, or "" before one is
// discovered (or while the process is down).
func (t *Tunnel) URL() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.url
}

func (t *Tunnel) setURL(u string) {
	t.mu.Lock()
	t.url = u
	t.mu.Unlock()
}

// Start supervises the tunnel until ctx is cancelled: it (re)spawns the binary,
// captures the public URL from its output, and restarts it with capped
// exponential backoff if it exits unexpectedly. Returns when ctx is done.
func (t *Tunnel) Start(ctx context.Context) {
	backoff := minBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		start := nowFunc()
		// While a run is live, the first URL line we see wins until the process
		// exits; a restart re-discovers it (it may change across reconnects).
		_ = t.run(ctx, t.cmd[0], t.cmd[1:], func(line string) {
			if u := extractURL(line); u != "" && t.URL() == "" {
				t.setURL(u)
				if t.onURL != nil {
					t.onURL(u)
				}
			}
		})
		// The process exited (or ctx cancelled). Drop the stale URL.
		t.setURL("")
		if ctx.Err() != nil {
			return
		}
		if nowFunc().Sub(start) >= healthyUptime {
			backoff = minBackoff // it ran a healthy while; treat the drop as fresh
		}
		select {
		case <-ctx.Done():
			return
		case <-afterFunc(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// nowFunc and afterFunc are time.Now / time.After, overridable so tests exercise
// the supervisor's restart/backoff path without real sleeping.
var (
	nowFunc   = time.Now
	afterFunc = time.After
)

// buildCommand resolves a Config into a command + args.
func buildCommand(cfg Config) ([]string, error) {
	if len(cfg.Command) > 0 {
		return cfg.Command, nil
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "cloudflare", "cloudflared":
		target := strings.TrimSpace(cfg.TargetURL)
		if target == "" {
			return nil, fmt.Errorf("tunnel: a target URL is required for the %q preset", cfg.Provider)
		}
		// Quick Tunnel: no account needed, prints an https://*.trycloudflare.com URL.
		return []string{"cloudflared", "tunnel", "--url", target}, nil
	case "ngrok":
		target := strings.TrimSpace(cfg.TargetURL)
		if target == "" {
			return nil, fmt.Errorf("tunnel: a target URL is required for the %q preset", cfg.Provider)
		}
		// ngrok wants a host:port (or addr), not a full URL; logfmt to stdout so we
		// can scan the "url=" field.
		return []string{"ngrok", "http", stripScheme(target), "--log=stdout", "--log-format=logfmt"}, nil
	case "custom":
		return nil, fmt.Errorf("tunnel: AGEZT_TUNNEL=custom requires AGEZT_TUNNEL_CMD")
	case "":
		return nil, fmt.Errorf("tunnel: set AGEZT_TUNNEL to a provider (cloudflare|cloudflared|ngrok) or AGEZT_TUNNEL_CMD to a command")
	default:
		return nil, fmt.Errorf("tunnel: unknown provider %q (use cloudflare, cloudflared, ngrok, or AGEZT_TUNNEL_CMD)", cfg.Provider)
	}
}

// stripScheme reduces http(s)://host:port[/path] to host:port for binaries (ngrok)
// that take an address rather than a URL.
func stripScheme(u string) string {
	s := u
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}

// urlRe matches an https URL up to the first whitespace/quote/bracket.
var urlRe = regexp.MustCompile(`https://[^\s"'<>|]+`)

// extractURL pulls the public tunnel URL out of one output line. It prefers a URL
// on a known tunnel domain (so a provider's banner link to its own docs isn't
// mistaken for the tunnel), falling back to the first https URL on the line.
func extractURL(line string) string {
	matches := urlRe.FindAllString(line, -1)
	if len(matches) == 0 {
		return ""
	}
	clean := func(s string) string { return strings.TrimRight(s, ".,);]") }
	for _, m := range matches {
		l := strings.ToLower(m)
		if strings.Contains(l, "trycloudflare.com") ||
			strings.Contains(l, "cfargotunnel.com") ||
			strings.Contains(l, "ngrok.io") ||
			strings.Contains(l, "ngrok-free.app") ||
			strings.Contains(l, "ngrok.app") ||
			strings.Contains(l, "ngrok.dev") {
			return clean(m)
		}
	}
	// A provider-docs/banner URL is unlikely to be the FIRST token on a URL line,
	// but if no known domain matched (a custom binary), the first https is our best
	// signal.
	return clean(matches[0])
}

// execRun is the production runFunc: it spawns the binary, scans its merged
// stdout+stderr line by line, and tears the whole process tree down when ctx is
// cancelled (cloudflared/ngrok are well-behaved, but a process-group kill is
// correct and cheap).
func execRun(ctx context.Context, name string, args []string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, name, args...)
	setProcessGroup(cmd)
	cmd.Cancel = func() error { killProcessTree(cmd); return nil }
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			onLine(sc.Text())
		}
		_ = sc.Err() // best-effort line capture; cmd.Wait() carries the real error
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()
	return cmd.Wait()
}
