// SPDX-License-Identifier: MIT

package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ghRoundTripper serves canned responses for GitHub-API requests so checkGitHub
// can be exercised without touching the network.
type ghRoundTripper struct {
	status int
	body   string
}

func (rt ghRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: rt.status,
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Header:     make(http.Header),
	}, nil
}

func newGitHubService(rt http.RoundTripper) *Service {
	return New(Config{
		Source:      SourceGitHub,
		GitHubOwner: "agezt",
		GitHubRepo:  "agezt",
		HTTPClient:  &http.Client{Transport: rt},
	})
}

func TestCheckInterval_And_DrainTimeout(t *testing.T) {
	svc := New(Config{
		Source:        SourceEndpoint,
		Endpoint:      "http://example.com",
		CheckInterval: 42 * time.Minute,
		DrainTimeout:  7 * time.Second,
	})
	if got := svc.CheckInterval(); got != 42*time.Minute {
		t.Errorf("CheckInterval() = %v, want 42m", got)
	}
	if got := svc.DrainTimeout(); got != 7*time.Second {
		t.Errorf("DrainTimeout() = %v, want 7s", got)
	}
}

func TestErrChecksumMismatch_Error(t *testing.T) {
	// Have/Want must be at least 8 chars — Error slices [:8].
	e := &ErrChecksumMismatch{Have: "deadbeefcafef00d", Want: "0123456789abcdef"}
	msg := e.Error()
	if !strings.Contains(msg, "deadbeef") || !strings.Contains(msg, "01234567") {
		t.Errorf("ErrChecksumMismatch.Error() = %q; want truncated have/want prefixes", msg)
	}
}

func TestCheck_UnknownSource(t *testing.T) {
	// An out-of-range Source value hits the default arm of Check's switch.
	svc := New(Config{Source: Source(99)})
	if _, err := svc.Check(context.Background()); err == nil {
		t.Fatal("Check() with an unknown source should return an error")
	}
}

func TestCheckGitHub_MissingOwnerRepo(t *testing.T) {
	svc := New(Config{Source: SourceGitHub}) // no owner/repo
	if _, err := svc.Check(context.Background()); err == nil {
		t.Fatal("checkGitHub with no owner/repo should error")
	}
}

func TestCheckGitHub_UpdateAvailable(t *testing.T) {
	orig := CurrentVersion
	CurrentVersion = "1.0.0"
	defer func() { CurrentVersion = orig }()

	// The asset name must contain "<os>_<arch>" (darwin normalised to macos) so
	// the platform matcher picks it regardless of the host running the test.
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macos"
	}
	assetName := "agezt-" + osName + "_" + runtime.GOARCH
	body := `{
		"tag_name": "v1.2.0",
		"body": "release notes",
		"assets": [
			{"name": "` + assetName + `", "browser_download_url": "https://example.com/` + assetName + `"}
		]
	}`
	svc := newGitHubService(ghRoundTripper{status: http.StatusOK, body: body})
	res, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Update == nil || res.Update.Version != "1.2.0" {
		t.Fatalf("Check() = %+v; want an update to 1.2.0", res)
	}
}

func TestCheckGitHub_NotModified(t *testing.T) {
	svc := newGitHubService(ghRoundTripper{status: http.StatusNotModified})
	res, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Update != nil {
		t.Errorf("304 response should yield no update, got %+v", res.Update)
	}
}

func TestCheckGitHub_BadStatus(t *testing.T) {
	svc := newGitHubService(ghRoundTripper{status: http.StatusInternalServerError, body: "boom"})
	if _, err := svc.Check(context.Background()); err == nil {
		t.Fatal("checkGitHub with a 500 status should return an error")
	}
}

func TestCheckGitHub_BadJSON(t *testing.T) {
	svc := newGitHubService(ghRoundTripper{status: http.StatusOK, body: "not json"})
	if _, err := svc.Check(context.Background()); err == nil {
		t.Fatal("checkGitHub with malformed JSON should return an error")
	}
}

// httpsRoundTripper serves a fixed body for any https download URL, letting
// downloadBinary + validateSHA256 run without a real TLS server.
type httpsRoundTripper struct {
	body []byte
}

func (rt httpsRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(rt.body)),
		Header:     make(http.Header),
	}, nil
}

func TestDownloadBinary_And_ValidateSHA256(t *testing.T) {
	payload := []byte("agezt-binary-payload-v1")
	sum := sha256.Sum256(payload)
	wantHex := hex.EncodeToString(sum[:])

	svc := New(Config{
		Source:     SourceEndpoint,
		HTTPClient: &http.Client{Transport: httpsRoundTripper{body: payload}},
	})

	dest := filepath.Join(t.TempDir(), "agezt-new")
	// requireHTTPS demands an https scheme; the RoundTripper ignores the host.
	if err := svc.downloadBinary(context.Background(), "https://example.com/agezt", dest); err != nil {
		t.Fatalf("downloadBinary error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("downloaded content mismatch")
	}

	// Correct checksum passes.
	if err := svc.validateSHA256(dest, wantHex); err != nil {
		t.Errorf("validateSHA256(correct) error = %v", err)
	}
	// Wrong checksum returns an ErrChecksumMismatch.
	badHex := strings.Repeat("00", 32)
	if err := svc.validateSHA256(dest, badHex); err == nil {
		t.Error("validateSHA256 with a wrong hash should error")
	}
	// Empty checksum is rejected.
	if err := svc.validateSHA256(dest, ""); err == nil {
		t.Error("validateSHA256 with an empty hash should error")
	}
}

func TestDownloadBinary_RejectsNonHTTPS(t *testing.T) {
	svc := New(Config{Source: SourceEndpoint, HTTPClient: &http.Client{Transport: httpsRoundTripper{body: []byte("x")}}})
	dest := filepath.Join(t.TempDir(), "bin")
	if err := svc.downloadBinary(context.Background(), "http://insecure.example/agezt", dest); err == nil {
		t.Fatal("downloadBinary should reject a non-HTTPS URL")
	}
}

// redirectRoundTripper returns a 302 to an HTTPS Location on the first call and
// a 200 with the payload thereafter, exercising downloadBinary's redirect arm.
type redirectRoundTripper struct {
	payload []byte
	calls   int
}

func (rt *redirectRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	rt.calls++
	if rt.calls == 1 {
		h := make(http.Header)
		h.Set("Location", "https://cdn.example.com/agezt-redirected")
		return &http.Response{
			StatusCode: http.StatusFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     h,
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(rt.payload)),
		Header:     make(http.Header),
	}, nil
}

func TestDownloadBinary_FollowsRedirect(t *testing.T) {
	payload := []byte("redirected-payload")
	svc := New(Config{Source: SourceEndpoint, HTTPClient: &http.Client{Transport: &redirectRoundTripper{payload: payload}}})
	dest := filepath.Join(t.TempDir(), "bin")
	if err := svc.downloadBinary(context.Background(), "https://example.com/agezt", dest); err != nil {
		t.Fatalf("downloadBinary(redirect) error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("redirected download content mismatch")
	}
}

// TestResolvePublicKey_DefaultHexFallback covers the branch where no runtime key
// is set but DefaultPublicKeyHex holds a valid Ed25519 key.
func TestResolvePublicKey_DefaultHexFallback(t *testing.T) {
	// Clear any runtime key and install a valid key via DefaultPublicKeyHex.
	_ = SetPublicKey("")
	prev := DefaultPublicKeyHex
	// 32 zero bytes is a structurally valid Ed25519 public key length.
	DefaultPublicKeyHex = strings.Repeat("00", 32)
	defer func() {
		DefaultPublicKeyHex = prev
		_ = SetPublicKey("")
	}()
	if got := resolvePublicKey(); got == nil {
		t.Fatal("resolvePublicKey should fall back to DefaultPublicKeyHex")
	}

	// A malformed DefaultPublicKeyHex falls through to nil.
	DefaultPublicKeyHex = "not-hex"
	if got := resolvePublicKey(); got != nil {
		t.Fatal("resolvePublicKey should return nil for a malformed DefaultPublicKeyHex")
	}
}
