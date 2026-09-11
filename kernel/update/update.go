// SPDX-License-Identifier: MIT

// Update core: constants + types (Config, Service, Source, UpdateInfo) + New + Check + CheckInterval + DrainTimeout.
// Code extracted from update.go during the Day-55 god-file split. Public API unchanged.
package update


import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/netguard"
)



// Source specifies where to fetch update metadata.
type Source int

const (
	SourceGitHub   Source = iota // GitHub Releases API
	SourceEndpoint               // Custom check.agezt.com-style endpoint
)

// Provenance records where an UpdateInfo CAME FROM, so the trust anchor is
// chosen by the manifest's own origin rather than by the service's configured
// Source.
//
// The distinction is load-bearing (UPD-001, 2026-08-12). verifySignature used
// to branch on s.cfg.Source, and with the shipping default (SourceGitHub, no
// embedded key) it accepted anything — reasoning that GitHub's TLS pipeline was
// the anchor. But the REST and control-plane handlers build an UpdateInfo
// entirely from a request body, so that premise silently failed: the URL never
// came from GitHub, and the SHA256 it was validated against was supplied by the
// same caller. An admin-token holder could stage an arbitrary binary over
// bin/agezt, surviving restart and token rotation.
//
// The zero value is deliberately the UNTRUSTED one. Source has SourceGitHub at
// iota 0, so a hand-built struct would have inherited the trusted origin by
// default — exactly the bug. A caller-assembled manifest now earns no trust it
// did not come by honestly.
type Provenance int

const (
	// ProvenanceUnverified is the zero value: a manifest assembled by a caller
	// (a REST body, a CLI flag, a test) rather than fetched from a release
	// source. Requires a signature under a configured key.
	ProvenanceUnverified Provenance = iota
	// ProvenanceGitHubRelease is set ONLY by checkGitHub, for a manifest whose
	// URL came from the GitHub Releases API over TLS.
	ProvenanceGitHubRelease
	// ProvenanceEndpoint is set ONLY by checkEndpoint. Signature verification is
	// mandatory for it: a self-supplied checksum is not a trust anchor.
	ProvenanceEndpoint
)

// UpdateInfo describes a available update.
type UpdateInfo struct {
	Version string // semver, e.g. "1.2.3"
	SHA256  string // lowercase hex SHA256 of the binary archive
	URL     string // direct download URL
	Notes   string // release notes (optional)
	// Provenance is where this manifest came from. Set by Check; left at the
	// zero value (ProvenanceUnverified) by anything that builds one by hand.
	Provenance Provenance
	// Signature is the hex Ed25519 signature over "<version>\n<sha256>",
	// attesting the release under the trusted public key (UPD-001). Empty
	// when the endpoint does not sign releases; Apply rejects an unsigned
	// release when a public key is configured.
	Signature string
}

// Manifest is the JSON shape returned by a custom update endpoint.
type Manifest struct {
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
	Notes     string `json:"notes,omitempty"`
	Signature string `json:"signature,omitempty"` // hex Ed25519 over "<version>\n<sha256>"
}

// Config tunes the update mechanism.
type Config struct {
	// Source selects the update check strategy.
	Source Source

	// GitHub owner/repo (used when Source == SourceGitHub).
	GitHubOwner string
	GitHubRepo  string

	// Endpoint URL (used when Source == SourceEndpoint).
	Endpoint string

	// SHA256 of the current binary; updates are skipped when the remote
	// version equals this (no self-downgrade).
	CurrentSHA256 string

	// Base directory; staging and lock files live under here.
	BaseDir string

	// DrainTimeout is how long to wait for in-flight runs to complete
	// before giving up. Zero means "do not drain, abort update".
	DrainTimeout time.Duration

	// CheckInterval is how often the background checker runs. Zero disables
	// background checking.
	CheckInterval time.Duration

	// HTTPClient is used for download requests. If nil, a default is used.
	HTTPClient *http.Client
}

// CheckResult is the outcome of a version check.
type CheckResult struct {
	Update  *UpdateInfo // nil if current version is up-to-date
	Current string      // brand.Version at time of check
	Err     error
}

// CurrentVersion returns brand.Version, factored out so this package has
// no import cycle with the brand package at runtime (the value is injected
// during Open).
var CurrentVersion = brand.Version

// Service is the update engine. A single instance lives in the daemon;
// it is safe for concurrent use after Open.
type Service struct {
	cfg        Config
	httpClient *http.Client
	transport  http.RoundTripper
}

// New returns a Service configured from cfg. The service does not
// perform any I/O until Check or Apply is called.
func New(cfg Config) *Service {
	hc := cfg.HTTPClient
	if hc == nil {
		const dialTimeout = 30 * time.Second
		// SSRF guard (CWE-918): screen every connection — the initial URL and
		// each redirect hop — through netguard so a malicious/redirected update
		// manifest URL can't be pointed at link-local/cloud-metadata or other
		// special-use ranges. The update source is operator-configured and may
		// legitimately be an internal mirror, so loopback and private ranges are
		// permitted (matching kernel/catalog/sync's posture); netguard still
		// blocks 169.254.0.0/16 (incl. 169.254.169.254 metadata), CGNAT,
		// broadcast and zero blocks. Defense-in-depth atop the HTTPS-every-hop
		// and SHA256+Ed25519 verification already enforced below.
		guard := netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate())
		hc = &http.Client{
			Timeout: dialTimeout,
			// Enforce TLS on EVERY redirect hop, not just the initial URL.
			// net/http follows redirects automatically; without a CheckRedirect
			// hook the manual requireHTTPS check in downloadBinary never runs
			// (the returned resp is already the final 200), so an HTTPS→HTTP
			// downgrade would be silently followed and the binary fetched over
			// plaintext. This hook refuses any non-TLS hop (loopback exempt) for
			// BOTH the manifest Check and the binary download (UPD-002).
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return requireHTTPS(req.URL.String())
			},
			Transport: &http.Transport{
				DialContext:         guard.Dialer(dialTimeout).DialContext,
				MaxIdleConns:        2,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		}
	}
	return &Service{
		cfg:        cfg,
		httpClient: hc,
		transport:  hc.Transport,
	}
}

// Check queries the configured source and returns whether an update is
// available. It returns (nil, nil) when the current version is up to date.
func (s *Service) Check(ctx context.Context) (*CheckResult, error) {
	switch s.cfg.Source {
	case SourceGitHub:
		return s.checkGitHub(ctx)
	case SourceEndpoint:
		return s.checkEndpoint(ctx)
	default:
		return nil, fmt.Errorf("update: unknown source %d", s.cfg.Source)
	}
}

// CheckInterval returns the configured periodic check interval.
// Zero means background checking is disabled.
func (s *Service) CheckInterval() time.Duration { return s.cfg.CheckInterval }

// DrainTimeout returns the configured drain timeout.
func (s *Service) DrainTimeout() time.Duration { return s.cfg.DrainTimeout }

// Apply orchestrates the drain → atomic swap → restart sequence.
// It returns ErrUpdateInProgress if Apply is already running.
//
// If DrainTimeout is zero the update aborts without modifying state
// (no drain attempted). If DrainTimeout > 0 the drain proceeds and
// Apply returns ErrDrainTimeout if in-flight work does not complete
// in time.
//
// On any error the current binary is left untouched and the daemon