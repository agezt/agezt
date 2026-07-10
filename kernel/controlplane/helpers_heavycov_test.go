// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"testing"
)

// TestArgTruthy_AllForms drives argTruthy through every branch: real bools, the
// accepted truthy string tokens (case/space-insensitive), rejected strings, and
// non-string/non-bool values (which are always false).
func TestArgTruthy_AllForms(t *testing.T) {
	truthy := []any{
		true,
		"on", "true", "yes", "1",
		"ON", "True", " Yes ", " 1 ",
	}
	for _, v := range truthy {
		if !argTruthy(v) {
			t.Errorf("argTruthy(%#v) = false, want true", v)
		}
	}

	falsy := []any{
		false,
		"off", "false", "no", "0", "", "maybe",
		nil,
		42,         // non-string number
		float64(1), // float is not accepted (only bool/string)
		[]string{"1"},
		map[string]any{"x": 1},
	}
	for _, v := range falsy {
		if argTruthy(v) {
			t.Errorf("argTruthy(%#v) = true, want false", v)
		}
	}
}

// TestTrimFloat covers both branches: whole-valued floats render without a
// trailing ".0", fractional floats keep their JSON form.
func TestTrimFloat(t *testing.T) {
	cases := map[float64]string{
		0:     "0",
		1:     "1",
		-5:    "-5",
		1000:  "1000",
		1.5:   "1.5",
		-2.25: "-2.25",
		0.5:   "0.5",
	}
	for in, want := range cases {
		if got := trimFloat(in); got != want {
			t.Errorf("trimFloat(%v) = %q, want %q", in, got, want)
		}
	}
}

// TestItoa checks the int64 → decimal-string helper.
func TestItoa(t *testing.T) {
	cases := map[int64]string{
		0:           "0",
		1:           "1",
		-1:          "-1",
		1234567890:  "1234567890",
		-9876543210: "-9876543210",
	}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestEnvOrDefault exercises both the set (non-empty) and unset/empty fallback
// branches of envOrDefault.
func TestEnvOrDefault(t *testing.T) {
	const name = "AGEZT_HEAVYCOV_ENV_PROBE"

	t.Setenv(name, "custom-value")
	if got := envOrDefault(name, "fallback"); got != "custom-value" {
		t.Errorf("envOrDefault set = %q, want %q", got, "custom-value")
	}

	// Empty value → fallback.
	t.Setenv(name, "")
	if got := envOrDefault(name, "fallback"); got != "fallback" {
		t.Errorf("envOrDefault empty = %q, want fallback", got)
	}

	// Unset entirely → fallback.
	os.Unsetenv(name)
	if got := envOrDefault(name, "fallback2"); got != "fallback2" {
		t.Errorf("envOrDefault unset = %q, want fallback2", got)
	}
}
