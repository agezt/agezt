// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestRequireHTTPS locks down the TLS-only policy for update URLs (UPD-002).
// Loopback http is exempt so local test harnesses / dev mirrors still work;
// everything else must be HTTPS or be refused.
func TestRequireHTTPS(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https", "https://check.agezt.com/v1/check", false},
		{"https plain host", "https://example.com/p", false},

		{"loopback http", "http://localhost:8080/p", false},
		{"loopback ip http", "http://127.0.0.1:8080/p", false},
		{"loopback ipv6 http", "http://[::1]:8080/p", false},

		{"non-loopback http refused", "http://example.com/p", true},
		{"ftp refused", "ftp://example.com/p", true},
		{"no scheme refused", "example.com/p", true},
		{"empty refused", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireHTTPS(tt.url)
			if tt.wantErr && err == nil {
				t.Errorf("requireHTTPS(%q): got nil, want error", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("requireHTTPS(%q): got %v, want nil", tt.url, err)
			}
		})
	}
}

// TestDownloadBinary_RefusesHTTPDowngradeRedirect proves an HTTPS (or loopback)
// URL that redirects to a non-TLS host is refused — the CheckRedirect hook
// enforces TLS on every hop, not just the initial URL. Without the hook, Go's
// client would follow the downgrade silently (the UPD-002 gap).
func TestDownloadBinary_RefusesHTTPDowngradeRedirect(t *testing.T) {
	// Loopback http server: the initial URL passes requireHTTPS (loopback
	// exempt), but it redirects to a NON-loopback http:// target.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/evil-bin", http.StatusFound)
	}))
	defer srv.Close()

	svc := New(Config{}) // default client → has the CheckRedirect hook

	dest := filepath.Join(t.TempDir(), "agezt")
	err := svc.downloadBinary(context.Background(), srv.URL, dest)
	if err == nil {
		t.Fatal("downloadBinary followed an HTTPS→HTTP downgrade; want refusal")
	}
	// The refusal must come from requireHTTPS, not a network/DNS error.
	if !strings.Contains(err.Error(), "non-HTTPS") && !strings.Contains(err.Error(), "scheme") {
		t.Errorf("downloadBinary error does not look like a TLS refusal: %v", err)
	}
}
