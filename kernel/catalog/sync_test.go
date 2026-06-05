// SPDX-License-Identifier: MIT

package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

func syncFrom(t *testing.T, body string) (*catalog.Catalog, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	s := catalog.NewSyncer()
	s.URL = srv.URL
	_, cat, _, err := s.Sync(context.Background())
	return cat, err
}

// TestSync_RejectsEmptyPayload: a syntactically valid but provider-less payload
// (`null` / `{}`, e.g. a proxy returning 200) must fail the sync so the caller never
// overwrites a good api.json with an empty catalog (M425).
func TestSync_RejectsEmptyPayload(t *testing.T) {
	for _, body := range []string{"null", "{}", "   "} {
		if _, err := syncFrom(t, body); err == nil {
			t.Errorf("sync of %q should fail (zero providers), got nil error", body)
		}
	}
}

// TestSync_AcceptsNonEmptyPayload: a payload with at least one provider syncs.
func TestSync_AcceptsNonEmptyPayload(t *testing.T) {
	cat, err := syncFrom(t, `{"anthropic":{"id":"anthropic","name":"Anthropic"}}`)
	if err != nil {
		t.Fatalf("valid single-provider sync should succeed: %v", err)
	}
	if len(cat.Providers) != 1 {
		t.Errorf("provider count = %d, want 1", len(cat.Providers))
	}
}
