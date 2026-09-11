// SPDX-License-Identifier: MIT

// Update verify path: verifySignature + checkGitHub + checkEndpoint + requireHTTPS + downloadBinary + validateSHA256 + acquireLock.
// Code extracted from update.go during the Day-55 god-file split. Public API unchanged.
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
	"runtime"
	"strings"
)


func (s *Service) verifySignature(info *UpdateInfo) error {
	pub := resolvePublicKey()
	if pub == nil {
		// No trusted key. Behaviour depends on where THIS MANIFEST came from —
		// not on how the service happens to be configured:
		//   - GitHub release: accept. The URL came from the Releases API over
		//     TLS, so GitHub's pipeline is the anchor and the manifest SHA256 is
		//     informational (still validated against the downloaded bytes).
		//   - Endpoint: refuse. A self-supplied checksum with no signature is
		//     exactly the vulnerability UPD-001 names — a compromised endpoint
		//     serves {malicious binary, matching sha256} and nothing checks it.
		//   - Unverified (the zero value): refuse. The manifest was assembled by
		//     a caller, so nothing about its URL is attested.
		if info.Provenance != ProvenanceGitHubRelease {
			return ErrSignatureKeyNotConfigured
		}
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
			Version:    version,
			URL:        downloadURL,
			Notes:      gh.Body,
			Provenance: ProvenanceGitHubRelease,
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
			Version:    m.Version,
			SHA256:     m.SHA256,
			URL:        m.URL,
			Notes:      m.Notes,
			Signature:  m.Signature,
			Provenance: ProvenanceEndpoint,
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
