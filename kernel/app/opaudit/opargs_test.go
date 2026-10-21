// SPDX-License-Identifier: MIT

package opaudit

import (
	"strings"
	"testing"
)

func TestIsSecretArg(t *testing.T) {
	secret := []string{
		"key", "api_key", "apikey", "TOKEN", "access_token", "secret", "client_secret",
		"password", "passwd", "passphrase", "cred", "credentials", "auth", "authorization",
		"cookie", "value", "private", "private_key", "signature", "code", "state",
	}
	for _, name := range secret {
		if !IsSecretArg(name) {
			t.Errorf("IsSecretArg(%q) = false, want true", name)
		}
	}
	plain := []string{"intent", "model", "agent", "limit", "path", "rating", "id"}
	for _, name := range plain {
		if IsSecretArg(name) {
			t.Errorf("IsSecretArg(%q) = true, want false", name)
		}
	}
}

func TestTruncateValue(t *testing.T) {
	short := "a short value"
	if got := TruncateValue(short); got != short {
		t.Errorf("TruncateValue = %q, want it unchanged", got)
	}
	long := strings.Repeat("x", ValueMax+10)
	got := TruncateValue(long)
	runes := []rune(got)
	if len(runes) != ValueMax+1 || runes[ValueMax] != '…' {
		t.Errorf("TruncateValue length = %d, want %d runes and an ellipsis", len(runes), ValueMax)
	}
	if !strings.HasPrefix(got, strings.Repeat("x", ValueMax)) {
		t.Errorf("TruncateValue = %q, want the head of the value", got)
	}
	// The bound is counted in runes, not bytes: a multi-byte value is not cut
	// in the middle of a character.
	multi := strings.Repeat("é", ValueMax+10)
	got = TruncateValue(multi)
	if len([]rune(got)) != ValueMax+1 {
		t.Errorf("TruncateValue runes = %d, want %d", len([]rune(got)), ValueMax)
	}
	if !strings.HasSuffix(got, "é…") {
		t.Errorf("TruncateValue = %q, want a whole character before the ellipsis", got)
	}
}

func TestSummarizeArgs(t *testing.T) {
	if got := SummarizeArgs(nil); got != nil {
		t.Errorf("SummarizeArgs(nil) = %v, want nil", got)
	}
	if got := SummarizeArgs(map[string]any{}); got != nil {
		t.Errorf("SummarizeArgs(empty) = %v, want nil", got)
	}
	out := SummarizeArgs(map[string]any{
		"intent": "do the thing",
		"limit":  10,
		"ratio":  1.5,
		"flag":   true,
		"key":    "sk-secret-value",
		"state":  "oauth-code",
		"list":   []any{"a", "b"},
		"nested": map[string]any{"b": 1, "a": 2},
		"other":  []string{"a"},
	})
	if out["intent"] != "do the thing" {
		t.Errorf("intent = %v", out["intent"])
	}
	if out["limit"] != 10 || out["ratio"] != 1.5 || out["flag"] != true {
		t.Errorf("scalars = %v", out)
	}
	if out["key"] != "[redacted]" || out["state"] != "[redacted]" {
		t.Errorf("secret args = %v", out)
	}
	if out["list"] != "[list]" {
		t.Errorf("list = %v", out["list"])
	}
	if out["nested"] != "[object: a,b]" {
		t.Errorf("nested = %v, want the sorted keys", out["nested"])
	}
	if out["other"] != "[value]" {
		t.Errorf("other = %v, want the shape of an unrecognized value", out["other"])
	}
}

// A nil argument is not journaled at all — there is nothing to say about it.
func TestSummarizeArgsDropsANilArgument(t *testing.T) {
	out := SummarizeArgs(map[string]any{"nothing": nil, "intent": "i"})
	if _, present := out["nothing"]; present {
		t.Errorf("summary = %v, want the nil argument dropped", out)
	}
	if out["intent"] != "i" {
		t.Errorf("summary = %v", out)
	}
}

// The summary never carries the secret itself.
func TestSummarizeArgsNeverCarriesTheSecret(t *testing.T) {
	out := SummarizeArgs(map[string]any{"api_key": "sk-abcdef", "password": "hunter2"})
	raw := ""
	for k, v := range out {
		raw += k + "=" + toString(v) + ";"
	}
	for _, secret := range []string{"sk-abcdef", "hunter2"} {
		if strings.Contains(raw, secret) {
			t.Errorf("summary leaked %q: %s", secret, raw)
		}
	}
}

// toString renders a summarized value for the leak check.
func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

// A long argument value is bounded in the summary too.
func TestSummarizeArgsBoundsAStringValue(t *testing.T) {
	out := SummarizeArgs(map[string]any{"intent": strings.Repeat("x", ValueMax+10)})
	got, _ := out["intent"].(string)
	if len([]rune(got)) != ValueMax+1 {
		t.Errorf("intent length = %d, want %d", len([]rune(got)), ValueMax+1)
	}
}
