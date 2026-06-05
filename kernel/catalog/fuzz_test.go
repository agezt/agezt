// SPDX-License-Identifier: MIT

package catalog_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

// FuzzParseAPIFile hardens the catalog parser against a malformed or hostile
// api.json — the daemon ingests this from the models.dev feed (`agt catalog sync`)
// and from disk, so it is an untrusted-external-input parser. Invariant:
// ParseAPIFile never panics on arbitrary bytes (it may return an error or a
// Catalog, but must not crash), and a successful parse yields a non-nil Catalog
// with a non-nil Providers map.
func FuzzParseAPIFile(f *testing.F) {
	f.Add([]byte(`{"anthropic":{"id":"anthropic","models":{"claude":{"id":"claude"}}}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"x":null}`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte("not json"))
	f.Add([]byte{0x00, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		cat, err := catalog.ParseAPIFile(data)
		if err != nil {
			return // malformed input rejected cleanly — fine
		}
		if cat == nil {
			t.Fatal("ParseAPIFile returned nil Catalog with nil error")
		}
		if cat.Providers == nil {
			t.Error("parsed Catalog has a nil Providers map")
		}
	})
}
