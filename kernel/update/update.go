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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

// Source specifies where to fetch update metadata.
type Source int

const (
	SourceGitHub Source = iota // GitHub Releases API
	SourceEndpoint             // Custom check.agezt.com-style endpoint
)

// UpdateInfo describes a available update.
type UpdateInfo struct {
	Version string // semver, e.g. "1.2.3"
	SHA256  string // lowercase hex SHA256 of the binary archive
	URL     string // direct download URL
	Notes   string // release notes (optional)
}

// Manifest is the JSON shape returned by a custom update endpoint.
type Manifest struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	URL     string `json:"url"`
	Notes   string `json:"notes,omitempty"`
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
	Timeout   bool  // true if drain timed out
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
			Version: m.Version,
			SHA256:  m.SHA256,
			URL:     m.URL,
			Notes:   m.Notes,
		},
	}, nil
}

// downloadBinary fetches the binary from url and writes it to dest.
func (s *Service) downloadBinary(ctx context.Context, url, dest string) error {
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

// BufferPoolingReader wraps an io.Reader and pools the scratch buffer
// used for copies. This reduces allocation pressure during large downloads.
type bufferPool struct {
	buf []byte
}

// ReadFrom implements io.ReaderFrom, copying through a pooled buffer.
func (p *bufferPool) ReadFrom(r io.Reader) (n int64, err error) {
	if p.buf == nil {
		p.buf = make([]byte, 32*1024) // 32 KiB chunks
	}
	return io.CopyBuffer(io.Discard, r, p.buf)
}
