// SPDX-License-Identifier: MIT

// Package updateapi is the contract the self-update engine and everyone who
// asks "is there a newer binary?" agree on.
//
// The engine verifies a release, drains the running work and swaps the
// binary — that is the module's work. The rows the answer is built from, the
// errors it reports and the port it is driven through belong here, for the
// same reason the toolbox's do (W5.4b) and the board's do (W3.59): the REST
// adapter and the control plane are ADAPTERS, and an adapter reaches the app
// layer's operations, never a module.
//
// The rules of the road:
//
//   - Rows only. No store, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
//
// The update module re-exports the rows under its own name, so the module,
// the daemon and the operations all speak the same type.
package updateapi

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Source specifies where to fetch update metadata.
type Source int

const (
	SourceGitHub   Source = iota // GitHub Releases API
	SourceEndpoint               // a custom update endpoint
)

// Provenance says how much a release manifest is trusted.
//
// The zero value is the unverified one on purpose: a manifest assembled by a
// caller earns no trust it did not come by honestly.
type Provenance int

const (
	// ProvenanceUnverified is the zero value: a manifest assembled by a caller
	// (a REST body, a CLI flag, a test) rather than fetched from a release
	// source. Requires a signature under a configured key.
	ProvenanceUnverified Provenance = iota
	// ProvenanceGitHubRelease is set ONLY for a manifest whose URL came from
	// the GitHub Releases API over TLS.
	ProvenanceGitHubRelease
	// ProvenanceEndpoint is set ONLY for a manifest fetched from a custom
	// endpoint. Signature verification is mandatory for it: a self-supplied
	// checksum is not a trust anchor.
	ProvenanceEndpoint
)

// UpdateInfo describes an available update.
type UpdateInfo struct {
	Version string // semver, e.g. "1.2.3"
	SHA256  string // lowercase hex SHA256 of the binary archive
	URL     string // direct download URL
	Notes   string // release notes

	// Provenance is the trust the manifest earned. A caller-assembled manifest
	// is ProvenanceUnverified even when it looks complete.
	Provenance Provenance
	// Signature is the detached signature over (version, sha256).
	Signature string
}

// Manifest is the JSON shape returned by a custom update endpoint.
type Manifest struct {
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
	Notes     string `json:"notes,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// CheckResult is the outcome of a version check.
type CheckResult struct {
	Update  *UpdateInfo // nil if current version is up-to-date
	Current string      // the running version at the time of the check
	Err     error
}

// DrainResult describes the outcome of the drain phase.
type DrainResult struct {
	Timeout    bool // true if drain timed out
	ActiveRuns int  // runs still in-flight when drain ended
}

// ErrDrainTimeout is returned when in-flight runs do not complete within the
// configured drain timeout.
var ErrDrainTimeout = errors.New("update: drain timed out — in-flight runs did not complete")

// ErrChecksumMismatch is returned when the downloaded binary's SHA256 does not
// match the manifest.
type ErrChecksumMismatch struct {
	Have string // hex of what was downloaded
	Want string // hex from the manifest
}

// Error describes the mismatch.
//
// The hexes are TRUNCATED, never sliced: the wanted hash comes from a
// manifest, and a manifest may carry a short one — a message that panics
// while reporting a failed checksum is a crash in the error path, and the
// error path is where a daemon is already unhappy.
func (e *ErrChecksumMismatch) Error() string {
	return fmt.Sprintf("update: SHA256 mismatch (have=%s, want=%s)", shortHex(e.Have), shortHex(e.Want))
}

// ErrSignatureInvalid is returned when the manifest's signature does not
// verify under the configured public key (wrong key, tampered version/hash,
// or a malformed signature).
type ErrSignatureInvalid struct{ Reason string }

func (e *ErrSignatureInvalid) Error() string {
	return "update: release signature is invalid: " + e.Reason
}

// shortHex is the first eight characters of a hex digest, or all of it when
// it is shorter.
func shortHex(hex string) string {
	if len(hex) <= 8 {
		return hex
	}
	return hex[:8]
}

// Backend is the update port: the surface the adapters drive. It is a port and
// not the update engine's methods because the control plane and the REST
// surface are ADAPTERS, and an adapter reaches the app layer's operations,
// never a module (archcheck's principle P2). The update module's *update.Service
// implements it.
type Backend interface {
	// Check looks for a newer release.
	Check(context.Context) (*CheckResult, error)
	// Apply verifies and installs one release, draining the running work
	// first through the given drain function.
	Apply(context.Context, *UpdateInfo, func(context.Context, time.Duration) DrainResult) error
}
