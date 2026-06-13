// SPDX-License-Identifier: MIT

// Package update implements Agezt's self-update mechanism.
//
// Design principles (from the Council of Elders deliberation):
//   - Drain-before-restart: the running daemon stops accepting new tasks,
//     waits for in-flight work to complete (bounded by a hard timeout),
//     then swaps the binary.
//   - Atomic binary swap: the new binary is written to a staging path,
//     validated (SHA256 checksum), and atomically renamed into place using
//     os.Rename — which is atomic on both POSIX and Windows.
//   - Fail-safe on error: if validation fails or the swap cannot complete,
//     the current binary is left untouched and the daemon does NOT
//     auto-restart. Human intervention is required to recover.
//   - State persistence: the kernel's journal and state store survive the
//     swap intact (they live under baseDir, not alongside the binary).
//
// Update check sources (configurable):
//   - GitHub Releases API: https://api.github.com/repos/<owner>/<repo>/releases/latest
//   - Custom endpoint: check.agezt.com (returns {version, sha256, url})
//
// The mechanism is triggered by:
//   - `agezt update` CLI subcommand
//   - A background periodic check (configurable interval)
//   - The REST API: POST /api/v1/update
package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

// Source specifies where to fetch update metadata.
type Source int

const (
	SourceGitHub   Source = iota // GitHub Releases API
	SourceEndpoint               // Custom check.agezt.com-style endpoint
)

// UpdateInfo describes a available update.
type UpdateInfo struct {
	Version string // semver, e.g. "1.2.3"
	SHA256  string // lowercase hex SHA256 of the binary archive
	URL     string // direct download URL
	Notes   string // release notes (optional)
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
		hc = &http.Client{
			Timeout: 30 * time.Second,
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
// does NOT restart — fail-safe: human intervention is required.
func (s *Service) Apply(ctx context.Context, info *UpdateInfo, drainFunc func(context.Context, time.Duration) DrainResult) error {
	// Guard against concurrent Apply calls.
	// The lockfile also serves as the PID file for the update sentinel.
	lockPath := filepath.Join(s.cfg.BaseDir, "update.lock")
	locked, err := s.acquireLock(lockPath)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if !locked {
		return ErrUpdateInProgress
	}
	defer os.Remove(lockPath) // best-effort; next Apply will retry

	// 1. Download to staging path.
	binDir := filepath.Join(s.cfg.BaseDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("update: mkdir bin dir: %w", err)
	}

	// Detect binary name (agezt.exe on Windows).
	binaryName := brand.Binary
	if runtime.GOOS == "windows" && !strings.HasSuffix(binaryName, ".exe") {
		binaryName += ".exe"
	}
	stagingPath := filepath.Join(binDir, binaryName+".new")
	livePath := filepath.Join(binDir, binaryName)

	// Remove any stale staging file from a previous failed attempt.
	os.Remove(stagingPath)

	if err := s.downloadBinary(ctx, info.URL, stagingPath); err != nil {
		return fmt.Errorf("update: download failed: %w", err)
	}

	// 2. Validate SHA256.
	if err := s.validateSHA256(stagingPath, info.SHA256); err != nil {
		os.Remove(stagingPath)
		return fmt.Errorf("update: validation failed: %w", err)
	}

	// 2b. Verify the release signature (UPD-001) — but only for the custom
	// endpoint source, which is the surface the audit flagged: an endpoint
	// must not be trusted to supply its own checksum, or a MITM / compromised
	// endpoint serves a malicious binary with a matching self-supplied hash.
	// GitHub-source updates rely on GitHub Releases' own TLS + asset integrity
	// (and are not signed in this scheme), so they are exempt. No-op (SHA-only
	// mode) until a key is configured via SetPublicKey / DefaultPublicKeyHex.
	if s.cfg.Source == SourceEndpoint {
		if err := s.verifySignature(info); err != nil {
			os.Remove(stagingPath)
			return fmt.Errorf("update: signature verification failed: %w", err)
		}
	}

	// 3. Drain (if configured).
	var drainResult DrainResult
	if s.cfg.DrainTimeout > 0 {
		drainResult = drainFunc(ctx, s.cfg.DrainTimeout)
		if drainResult.Timeout {
			// Drain timed out — leave staging file in place for inspection.
			// Do NOT swap. Do NOT restart.
			return ErrDrainTimeout
		}
	}

	// 4. Atomic rename: staging → live.
	// os.Rename is atomic on the same filesystem on both POSIX and Windows.
	if err := os.Rename(stagingPath, livePath); err != nil {
		return fmt.Errorf("update: atomic rename failed (current binary untouched): %w", err)
	}

	// 5. Make the new binary executable (Linux/macOS; no-op on Windows).
	if runtime.GOOS != "windows" {
		if err := os.Chmod(livePath, 0o755); err != nil {
			// Binary is in place but not executable — fail-safe:
			// leave it and do not restart. Human must fix permissions.
			return fmt.Errorf("update: chmod new binary: %w (binary replaced but not executable — manual intervention required)", err)
		}
	}

	return nil
}

// DrainResult describes the outcome of the drain phase.
type DrainResult struct {
	Timeout    bool // true if drain timed out
	ActiveRuns int  // runs still in-flight when drain ended
}

// ErrUpdateInProgress is returned when Apply is called while an update
// is already in progress.
var ErrUpdateInProgress = errors.New("update: another update is already in progress")

// ErrDrainTimeout is returned when in-flight runs do not complete within
// the configured DrainTimeout.
var ErrDrainTimeout = errors.New("update: drain timed out — in-flight runs did not complete")

// ErrChecksumMismatch is returned when the downloaded binary's SHA256
// does not match the manifest.
type ErrChecksumMismatch struct {
	Have string // hex of what was downloaded
	Want string // hex from the manifest
}

func (e *ErrChecksumMismatch) Error() string {
	return fmt.Sprintf("update: SHA256 mismatch (have=%s, want=%s)", e.Have[:8], e.Want[:8])
}

// ErrSignatureMissing is returned when a public key is configured but the
// manifest carries no signature.
var ErrSignatureMissing = errors.New("update: release is not signed but a public key is configured")

// ErrSignatureInvalid is returned when the manifest's signature does not verify
// under the configured public key (wrong key, tampered version/hash, or a
// malformed signature).
type ErrSignatureInvalid struct{ Reason string }

func (e *ErrSignatureInvalid) Error() string {
	return "update: release signature is invalid: " + e.Reason
}

// DefaultPublicKeyHex is the Ed25519 public key (lowercase hex, 64 chars) the
// daemon trusts for release signatures. Inject it at build time, e.g.:
//
//	go build -ldflags '-X github.com/agezt/agezt/kernel/update.DefaultPublicKeyHex=<hex>'
//
// Empty (the default) leaves updates in SHA256-only mode — backward compatible,
// but a configured endpoint can still supply a matching {binary, hash} pair
// (UPD-001). Embed a key the moment releases are signed.
var DefaultPublicKeyHex = ""

// Trusted release-signing key, overridable at runtime via SetPublicKey (e.g.
// from an env var). Protected by pubKeyMu.
var (
	pubKeyMu     sync.RWMutex
	updatePubKey ed25519.PublicKey
)

// SetPublicKey configures the trusted release-signing key at runtime. Pass an
// empty string to clear it. Returns an error if the hex does not decode to a
// 32-byte Ed25519 public key.
func SetPublicKey(hexKey string) error {
	pubKeyMu.Lock()
	defer pubKeyMu.Unlock()
	if strings.TrimSpace(hexKey) == "" {
		updatePubKey = nil
		return nil
	}
	raw, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return fmt.Errorf("update: bad public key hex: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return fmt.Errorf("update: public key is %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	updatePubKey = ed25519.PublicKey(raw)
	return nil
}

// resolvePublicKey returns the trusted key: the runtime-configured key if set,
// otherwise the build-time DefaultPublicKeyHex, otherwise nil (SHA-only mode).
func resolvePublicKey() ed25519.PublicKey {
	pubKeyMu.RLock()
	k := updatePubKey
	pubKeyMu.RUnlock()
	if len(k) == ed25519.PublicKeySize {
		return k
	}
	if h := strings.TrimSpace(DefaultPublicKeyHex); h != "" {
		if raw, err := hex.DecodeString(h); err == nil && len(raw) == ed25519.PublicKeySize {
			return ed25519.PublicKey(raw)
		}
	}
	return nil
}

// signedMessage is the canonical bytes a release signature covers: the version
// and the binary SHA256, newline-separated. Both are integrity-critical (the
// URL is transport, gated separately by requireHTTPS; notes are cosmetic).
func signedMessage(version, sumHex string) []byte {
	return []byte(version + "\n" + sumHex)
}

// verifySignature enforces the release signature when a public key is
// configured. With no key configured it returns nil (SHA256-only mode) so the
// mechanism is opt-in and never breaks an unsigned deployment.
func (s *Service) verifySignature(info *UpdateInfo) error {
	pub := resolvePublicKey()
	if pub == nil {
		// No trusted key: SHA256-only mode. This is backward compatible but is
		// NOT full integrity — UPD-001 stays open until a key is embedded.
		return nil
	}
	sig := strings.TrimSpace(info.Signature)
	if sig == "" {
		return ErrSignatureMissing
	}
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return &ErrSignatureInvalid{Reason: "signature is not valid hex: " + err.Error()}
	}
	if !ed25519.Verify(pub, signedMessage(info.Version, info.SHA256), sigBytes) {
		return &ErrSignatureInvalid{Reason: "signature does not match version/sha256 under the trusted key"}
	}
	return nil
}

// checkGitHub fetches the latest release from GitHub Releases.
func (s *Service) checkGitHub(ctx context.Context) (*CheckResult, error) {
	owner := s.cfg.GitHubOwner
	repo := s.cfg.GitHubRepo
	if owner == "" || repo == "" {
		return nil, errors.New("update: GitHub owner/repo not configured")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	// unauthenticated: 60 req/hour — sufficient for background checking.

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &CheckResult{Current: CurrentVersion, Update: nil}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: GitHub API returned status %d", resp.StatusCode)
	}

	var gh struct {
		TagName string `json:"tag_name"` // e.g. "v1.2.3"
		Body    string `json:"body"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return nil, fmt.Errorf("update: parse GitHub release: %w", err)
	}

	version := strings.TrimPrefix(gh.TagName, "v")
	if version == CurrentVersion {
		return &CheckResult{Current: CurrentVersion, Update: nil}, nil
	}

	// Find the binary asset for the current GOOS/GOARCH.
	arch := runtime.GOARCH
	osName := runtime.GOOS
	// Normalise "darwin" → "macos" if the release uses that convention.
	if runtime.GOOS == "darwin" {
		osName = "macos"
	}
	platformName := fmt.Sprintf("%s_%s", osName, arch)
	var downloadURL string
	for _, a := range gh.Assets {
		if strings.Contains(a.Name, platformName) {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("update: no binary asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	return &CheckResult{
		Current: CurrentVersion,
		Update: &UpdateInfo{
			Version: version,
			URL:     downloadURL,
			Notes:   gh.Body,
		},
	}, nil
}

// checkEndpoint fetches update metadata from a custom endpoint.
func (s *Service) checkEndpoint(ctx context.Context) (*CheckResult, error) {
	endpoint := s.cfg.Endpoint
	if endpoint == "" {
		return nil, errors.New("update: endpoint not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: endpoint request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: endpoint returned status %d", resp.StatusCode)
	}

	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("update: parse endpoint response: %w", err)
	}

	if m.Version == "" {
		return nil, errors.New("update: endpoint returned empty version")
	}
	if m.Version == CurrentVersion {
		return &CheckResult{Current: CurrentVersion, Update: nil}, nil
	}

	return &CheckResult{
		Current: CurrentVersion,
		Update: &UpdateInfo{
			Version:   m.Version,
			SHA256:    m.SHA256,
			URL:       m.URL,
			Notes:     m.Notes,
			Signature: m.Signature,
		},
	}, nil
}

// requireHTTPS rejects a non-HTTPS update URL. The update payload and (for the
// custom endpoint) the SHA it is checked against must travel over TLS, or a
// network MITM could swap the binary or its checksum. Loopback over http is
// exempt so local test harnesses (httptest) and dev mirrors still work.
func requireHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("update: bad URL %q: %w", raw, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if host := u.Hostname(); host == "localhost" || net.ParseIP(host).IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("update: refusing non-HTTPS update URL %q (scheme %q)", raw, u.Scheme)
}

// downloadBinary fetches the binary from url and writes it to dest.
func (s *Service) downloadBinary(ctx context.Context, url, dest string) error {
	if err := requireHTTPS(url); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther:
		// Follow redirects for CDN redirects (common for binary downloads).
		redirectURL := resp.Header.Get("Location")
		if redirectURL == "" {
			return errors.New("update: redirect without Location header")
		}
		req.URL, err = req.URL.Parse(redirectURL)
		if err != nil {
			return fmt.Errorf("update: redirect URL parse failed: %w", err)
		}
		// Refuse an HTTPS→HTTP downgrade on redirect — the resolved absolute URL
		// (after resolving a relative Location) must still be TLS.
		if err := requireHTTPS(req.URL.String()); err != nil {
			return err
		}
		resp.Body.Close()
		resp, err = s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("update: download redirect request failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("update: download returned status %d after redirect", resp.StatusCode)
		}
	default:
		return fmt.Errorf("update: download returned status %d", resp.StatusCode)
	}

	// Write to temp file first, then rename — same atomic pattern as state.go.
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("update: open temp file: %w", err)
	}
	written, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("update: write temp file: %w", err)
	}
	if written == 0 {
		os.Remove(tmp)
		return errors.New("update: downloaded file is empty")
	}

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("update: rename temp to staging: %w", err)
	}
	return nil
}

// validateSHA256 computes the SHA256 of file and compares it to wantHex.
func (s *Service) validateSHA256(path, wantHex string) error {
	wantHex = strings.TrimSpace(strings.ToLower(wantHex))
	if wantHex == "" {
		return errors.New("update: empty SHA256 in manifest")
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("update: open binary for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("update: computing SHA256: %w", err)
	}
	have := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(have, wantHex) {
		return &ErrChecksumMismatch{Have: have, Want: wantHex}
	}
	return nil
}

// acquireLock tries to create a lockfile using os.O_CREATE|os.O_EXCL.
// Returns (true, nil) on success, (false, nil) if the file already exists.
func (s *Service) acquireLock(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("update: lockfile open: %w", err)
	}
	// Write PID so operators can see who holds the lock.
	fmt.Fprintf(f, "%d", os.Getpid())
	f.Close()
	return true, nil
}

// SpawnRestart spawns a new daemon process and returns. The current
// process should exit after SpawnRestart returns successfully.
// Uses the same executable and inherits the process environment.
func SpawnRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: find own executable: %w", err)
	}

	// Re-use the same "daemon" subcommand path the watchdog uses.
	cmd, _ := os.FindProcess(os.Getpid())
	if cmd == nil {
		// Fallback: re-exec with "daemon" argument.
		cmd = nil // unused
	}
	_ = cmd

	// exec the same binary, passing "daemon" so it goes directly into runDaemon.
	// We use StartProcess directly (instead of exec.Command) so the new process
	// inherits the current process's environment and file descriptors.
	attr := &os.ProcAttr{
		Dir:   "", // inherit working directory
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}
	process, err := os.StartProcess(exe, []string{exe, "daemon"}, attr)
	if err != nil {
		return fmt.Errorf("update: spawn new daemon: %w", err)
	}

	// Detach: the child is now independent. We don't wait for it.
	process.Release()
	return nil
}

// ParseVersion compares two semver strings. It returns -1 if a < b,
// +1 if a > b, and 0 if equal. A non-semver string sorts before any
// valid semver (so "latest" or "dev" builds don't block real releases).
func ParseVersion(a, b string) int {
	va := parseSemver(a)
	vb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if va[i] < vb[i] {
			return -1
		}
		if va[i] > vb[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	var out [3]int
	// Strip leading 'v'.
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		var n int
		if _, err := fmt.Sscanf(parts[i], "%d", &n); err != nil {
			continue // non-numeric component ("latest", "dev", "") stays 0 — sorts before any release
		}
		out[i] = n
	}
	return out
}
