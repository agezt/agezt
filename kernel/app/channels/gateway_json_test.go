// SPDX-License-Identifier: MIT
package channels

import (
	"encoding/json"
	"testing"
)

// Keep original map-shaped wire assertions independent of DTO construction.
func gatewayJSONView(t *testing.T, output any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if code, ok := result["http_status"].(float64); ok {
		result["http_status"] = int(code)
	}
	return result
}
