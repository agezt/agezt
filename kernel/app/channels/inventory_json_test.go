// SPDX-License-Identifier: MIT
package channels

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
)

// Keep the original map-shaped wire assertions while the production models become
// typed. Only test values are normalized; native parity compares raw bytes.
func inventoryJSONView(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var parsed any
	if err := decoder.Decode(&parsed); err != nil {
		t.Fatal(err)
	}
	var normalize func(any) any
	normalize = func(value any) any {
		switch v := value.(type) {
		case json.Number:
			n, err := strconv.Atoi(v.String())
			if err != nil {
				t.Fatal(err)
			}
			return n
		case map[string]any:
			for key, item := range v {
				v[key] = normalize(item)
			}
			return v
		case []any:
			rows := make([]map[string]any, 0, len(v))
			objects := true
			for i, item := range v {
				v[i] = normalize(item)
				if row, ok := v[i].(map[string]any); ok {
					rows = append(rows, row)
				} else {
					objects = false
				}
			}
			if objects {
				return rows
			}
			return v
		default:
			return value
		}
	}
	return normalize(parsed).(map[string]any)
}
