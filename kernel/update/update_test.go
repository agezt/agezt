// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestService_checkEndpoint(t *testing.T) {
	// Set CurrentVersion so the test is deterministic.
	orig := CurrentVersion
	CurrentVersion = "1.0.0"
	defer func() { CurrentVersion = orig }()

	t.Run("up to date", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(Manifest{Version: "1.0.0", SHA256: "abc123", URL: "http://example.com/b"})
		}))
		defer srv.Close()

		svc := New(Config{
			Source:   SourceEndpoint,
			Endpoint: srv.URL,
		})
		result, err := svc.Check(context.Background())
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if result.Update != nil {
			t.Errorf("up to date: Update = %+v; want nil", result.Update)
		}
	})

	t.Run("update available", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(Manifest{
				Version: "1.1.0",
				SHA256:  "deadbeef",
				URL:     "http://example.com/agezt-1.1.0",
				Notes:   "bug fixes",
			})
		}))
		defer srv.Close()

		svc := New(Config{
			Source:   SourceEndpoint,
			Endpoint: srv.URL,
		})
		result, err := svc.Check(context.Background())
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if result.Update == nil {
			t.Fatalf("Update available: Update = nil; want non-nil")
		}
		if result.Update.Version != "1.1.0" {
			t.Errorf("Update.Version = %q; want %q", result.Update.Version, "1.1.0")
		}
		if result.Update.URL != "http://example.com/agezt-1.1.0" {
			t.Errorf("Update.URL = %q; want %q", result.Update.URL, "http://example.com/agezt-1.1.0")
		}
	})

	t.Run("endpoint error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		svc := New(Config{
			Source:   SourceEndpoint,
			Endpoint: srv.URL,
		})
		_, err := svc.Check(context.Background())
		if err == nil {
			t.Error("Check() error = nil; want non-nil")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()

		svc := New(Config{
			Source:   SourceEndpoint,
			Endpoint: srv.URL,
		})
		_, err := svc.Check(context.Background())
		if err == nil {
			t.Error("Check() error = nil; want non-nil")
		}
	})

	t.Run("empty version in response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(Manifest{Version: "", SHA256: "abc", URL: "http://x.com"})
		}))
		defer srv.Close()

		svc := New(Config{
			Source:   SourceEndpoint,
			Endpoint: srv.URL,
		})
		_, err := svc.Check(context.Background())
		if err == nil {
			t.Error("Check() error = nil; want error for empty version")
		}
	})
}

func TestService_validateSHA256(t *testing.T) {
	// Create a temp file with known content.
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// SHA256 of "hello world".
	want := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	svc := New(Config{})

	t.Run("valid checksum", func(t *testing.T) {
		if err := svc.validateSHA256(path, want); err != nil {
			t.Errorf("validateSHA256: %v; want nil", err)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		if err := svc.validateSHA256(path, "B94D27B9934D3E08A52E52D7DA7DABFAC484EFE37A5380EE9088F7ACE2EFCDE9"); err != nil {
			t.Errorf("validateSHA256 (uppercase): %v; want nil", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		err := svc.validateSHA256(path, "0000000000000000000000000000000000000000000000000000000000000000")
		if err == nil {
			t.Error("validateSHA256: nil; want ErrChecksumMismatch")
		}
		var mismatch *ErrChecksumMismatch
		if ok := errors.As(err, &mismatch); !ok {
			t.Errorf("error type = %T; want *ErrChecksumMismatch", err)
		}
	})

	t.Run("empty checksum", func(t *testing.T) {
		err := svc.validateSHA256(path, "")
		if err == nil {
			t.Error("validateSHA256: nil; want error for empty checksum")
		}
	})
}

func TestService_acquireLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock")

	svc := New(Config{})

	// First acquire should succeed.
	locked, err := svc.acquireLock(path)
	if err != nil {
		t.Fatalf("acquireLock #1: %v", err)
	}
	if !locked {
		t.Error("acquireLock #1: locked = false; want true")
	}

	// Second acquire from same service should fail (same process).
	locked2, err := svc.acquireLock(path)
	if err != nil {
		t.Fatalf("acquireLock #2: %v", err)
	}
	if locked2 {
		t.Error("acquireLock #2: locked = true; want false (already held)")
	}

	// Remove lock and retry.
	os.Remove(path)
	locked3, err := svc.acquireLock(path)
	if err != nil {
		t.Fatalf("acquireLock #3: %v", err)
	}
	if !locked3 {
		t.Error("acquireLock #3: locked = false; want true after remove")
	}
	os.Remove(path)
}

func TestService_Check_context_cancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // deliberately slow
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := New(Config{
		Source:     SourceEndpoint,
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Timeout: 100 * time.Millisecond},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := svc.Check(ctx)
	if err == nil {
		t.Error("Check() error = nil; want context deadline exceeded")
	}
}
