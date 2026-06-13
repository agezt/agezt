// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)

// intOf decodes a JSON number (always float64 after Go's stdlib
// decoder) back to int. Mirrors mcOf in budget_test.go but for
// non-microcent counts.
func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return -1
}

// mcOf is a helper that accepts the float64 / int64 ambiguity of
// JSON-decoded numbers. Mirrors the cmd/agt/budget.go helper.
func mcOf(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// dialUntilReady polls until the runtime files are on disk, then
// returns a connected client. Used by tests that need custom kernel
// setup beyond what startPair provides.
func dialUntilReady(t *testing.T, dir string) (*controlplane.Client, error) {
	t.Helper()
	var (
		client  *controlplane.Client
		lastErr error
	)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := controlplane.NewClient(dir)
		if err == nil {
			client = c
			break
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	if client == nil {
		return nil, lastErr
	}
	return client, nil
}
