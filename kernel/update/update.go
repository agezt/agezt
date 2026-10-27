// SPDX-License-Identifier: MIT

package update

// Provenance: Update core: constants + types (Config, Service, Source, UpdateInfo) +
//             New + Check + CheckInterval. Code extracted from
//             update.go during the Day-55 god-file split. Public API unchanged.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/agezt/agezt/kernel/contract/updateapi"
	"github.com/agezt/agezt/kernel/platform/netout"
)

// Source specifies where to fetch update metadata.
//
// The row lives in the contract (W5.4c): the REST adapter and the control
// plane are adapters, and an adapter may not name this module.
type Source = updateapi.Source

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
//
// The row itself lives in the contract (W5.4c).
type Provenance = updateapi.Provenance

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
//
// The row itself lives in the contract (W5.4c) — this is an ALIAS, so there
// is one row and one spelling of it.
type UpdateInfo = updateapi.UpdateInfo

// Manifest is the JSON shape returned by a custom update endpoint.
//
// The row itself lives in the contract (W5.4c).
type Manifest = updateapi.Manifest

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
//
// The row itself lives in the contract (W5.4c).
type CheckResult = updateapi.CheckResult

// The running binary's version is the brand's (`internal/brand.Version`, the
// value the build stamps). This package used to keep a second copy of it — a
// var set to brand.Version at init and changed only by tests — and the
// adapters read that copy while the module compared against it, so the truth
// existed twice and could drift.
//
// There is now ONE current version in the tree. The brand's version is a var,
// so a test that needs determinism pins brand.Version (and restores it) —
// exactly as it pinned the old copy.
//
// The rows that carry the version (CheckResult.Current) are the contract's.

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
		const requestTimeout = 30 * time.Second
		// SSRF guard (CWE-918): the operator client screens every connection —
		// the initial URL and each redirect hop — so a malicious/redirected
		// update manifest URL can't be pointed at link-local/cloud-metadata or
		// other special-use ranges. The update source is operator-configured
		// and may legitimately be an internal mirror, so loopback and private
		// ranges stay reachable (kernel/platform/netout). Defense-in-depth atop
		// the HTTPS-every-hop and SHA256+Ed25519 verification enforced below.
		hc = netout.OperatorClient(requestTimeout)
		// Enforce TLS on EVERY redirect hop, not just the initial URL.
		// net/http follows redirects automatically; without a CheckRedirect
		// hook the manual requireHTTPS check in downloadBinary never runs
		// (the returned resp is already the final 200), so an HTTPS→HTTP
		// downgrade would be silently followed and the binary fetched over
		// plaintext. This hook refuses any non-TLS hop (loopback exempt) for
		// BOTH the manifest Check and the binary download (UPD-002).
		hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return requireHTTPS(req.URL.String())
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

// Apply orchestrates the drain → atomic swap → restart sequence.
// It returns ErrUpdateInProgress if Apply is already running.
//
// The download is verified (SHA-256, then signature) BEFORE the drain
// runs. If DrainTimeout is zero no drain is attempted and the swap
// follows verification directly. If DrainTimeout > 0 the drain proceeds
// and Apply returns ErrDrainTimeout if in-flight work does not complete
// in time.
//
// On any error the current binary is left untouched and the daemon
