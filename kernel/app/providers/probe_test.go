// SPDX-License-Identifier: MIT

package providers_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app/providers"
)

func TestProbeServicePreservesReachabilityAuthorizationAndModelCount(t *testing.T) {
	for _, tc := range []struct {
		code       int
		body       string
		reachable  bool
		authorized bool
		models     int
	}{
		{200, `{"data":[{"id":"one"},{"id":"two"}]}`, true, true, 2},
		{204, ``, true, true, 0},
		{200, `{`, true, true, 0},
		{401, `{"data":[{"id":"ignored"}]}`, true, false, 0},
		{403, `{"data":[{"id":"ignored"}]}`, true, false, 0},
		{404, `{}`, false, false, 0},
		{503, `{}`, false, false, 0},
	} {
		calls := 0
		svc := providers.NewProbe(func(endpoint, header, key string, max int64) ([]byte, int, string, error) {
			calls++
			if endpoint != "https://fixture.invalid/v1/models" || header != "Authorization" || key != "Bearer fixture" || max != 1<<20 {
				t.Fatalf("transport input changed: %q %q %q %d", endpoint, header, key, max)
			}
			return []byte(tc.body), tc.code, "application/json", nil
		})
		out, err := svc.Check(providers.ProbeInput{URL: " https://fixture.invalid/v1/// ", Key: " fixture "})
		want := map[string]any{"ok": true, "reachable": tc.reachable, "authorized": tc.authorized, "http_status": tc.code, "models": tc.models}
		if err != nil || calls != 1 || !reflect.DeepEqual(probeWire(t, out), want) {
			t.Fatalf("status %d = %v, %v, calls=%d; want %v", tc.code, out, err, calls, want)
		}
	}
}

func TestProbeServicePreservesMissingURLKeyAndFailureShapes(t *testing.T) {
	calls := 0
	cause := errors.New("fixture transport unavailable")
	svc := providers.NewProbe(func(endpoint, header, key string, max int64) ([]byte, int, string, error) {
		calls++
		if endpoint != "https://fixture.invalid/models" || header != "Authorization" || key != "" || max != 1<<20 {
			t.Fatalf("keyless transport input changed: %q %q %q %d", endpoint, header, key, max)
		}
		return nil, 0, "", cause
	})
	out, err := svc.Check(providers.ProbeInput{URL: "  "})
	if err == nil || err.Error() != "args.url is required" || !reflect.ValueOf(out).IsZero() || calls != 0 {
		t.Fatalf("missing URL entered transport: %v, %v, calls=%d", out, err, calls)
	}
	out, err = svc.Check(providers.ProbeInput{URL: "https://fixture.invalid", Key: " "})
	want := map[string]any{"ok": false, "error": "cannot reach endpoint: fixture transport unavailable"}
	if err != nil || calls != 1 || !reflect.DeepEqual(probeWire(t, out), want) {
		t.Fatalf("transport failure shape changed: %v %v calls=%d", out, err, calls)
	}
}

func probeWire(t *testing.T, value providers.ProbeOutput) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if status, ok := out["http_status"].(float64); ok {
		out["http_status"] = int(status)
	}
	if models, ok := out["models"].(float64); ok {
		out["models"] = int(models)
	}
	return out
}
