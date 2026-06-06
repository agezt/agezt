// SPDX-License-Identifier: MIT

package openaiapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/journal"
)

// usageEngine wraps fakeEngine and reports real provider usage, exercising the
// UsageReporter path (M282).
type usageEngine struct {
	*fakeEngine
	pt, ct int
	ok     bool
}

func (u *usageEngine) UsageFor(string) (int, int, bool) { return u.pt, u.ct, u.ok }

func chatUsageBlock(t *testing.T, eng Engine) map[string]any {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	if fe, ok := eng.(*fakeEngine); ok {
		fe.b = b
	}
	if ue, ok := eng.(*usageEngine); ok {
		ue.fakeEngine.b = b
	}
	s := New(eng, b, "secret")

	body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"name three primary colors"}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	u, _ := out["usage"].(map[string]any)
	if u == nil {
		t.Fatal("response missing usage")
	}
	return u
}

// When the engine reports real usage, the API surfaces the provider's token
// counts (not the whitespace estimate).
func TestChatUsage_RealProviderTokens(t *testing.T) {
	u := chatUsageBlock(t, &usageEngine{
		fakeEngine: &fakeEngine{answer: "Red, blue, yellow.", model: "gpt-5.5"},
		pt:         1406, ct: 11, ok: true,
	})
	if pt, _ := u["prompt_tokens"].(float64); pt != 1406 {
		t.Errorf("prompt_tokens = %v, want 1406 (real provider usage)", u["prompt_tokens"])
	}
	if ct, _ := u["completion_tokens"].(float64); ct != 11 {
		t.Errorf("completion_tokens = %v, want 11", u["completion_tokens"])
	}
	if tot, _ := u["total_tokens"].(float64); tot != 1417 {
		t.Errorf("total_tokens = %v, want 1417", u["total_tokens"])
	}
}

// When the engine has no usage to report (ok=false) or doesn't implement the
// reporter, the API falls back to the whitespace estimate — never 0/0.
func TestChatUsage_FallsBackToEstimate(t *testing.T) {
	// ok=false → fall back.
	u := chatUsageBlock(t, &usageEngine{
		fakeEngine: &fakeEngine{answer: "Red, blue, yellow.", model: "gpt-5.5"},
		ok:         false,
	})
	// "name three primary colors" = 4 whitespace tokens for the prompt.
	if pt, _ := u["prompt_tokens"].(float64); pt != 4 {
		t.Errorf("prompt_tokens = %v, want the 4-word estimate", u["prompt_tokens"])
	}
	if tot, _ := u["total_tokens"].(float64); tot == 0 {
		t.Error("estimate fallback should never report total 0")
	}

	// A plain engine that doesn't implement UsageReporter also falls back.
	u2 := chatUsageBlock(t, &fakeEngine{answer: "Red.", model: "gpt-5.5"})
	if pt, _ := u2["prompt_tokens"].(float64); pt != 4 {
		t.Errorf("non-reporter prompt_tokens = %v, want the estimate", u2["prompt_tokens"])
	}
}

// TestEstimateUsage_WordCount pins the word-count usage fallback used when the engine is
// not a UsageReporter (no real provider token counts). usage_test.go's main test uses a
// UsageReporter engine, so it exercises chatUsage's `pt + ct` path, never estimateUsage —
// leaving its `total_tokens: p + c` arithmetic unpinned (mutation M527 showed `+ → *` and
// `+ → -` survived). prompt/completion are the whitespace-field counts; total is their sum.
func TestEstimateUsage_WordCount(t *testing.T) {
	u := estimateUsage("one two three", "four five")
	if got, _ := u["prompt_tokens"].(int); got != 3 {
		t.Errorf("prompt_tokens = %v, want 3", u["prompt_tokens"])
	}
	if got, _ := u["completion_tokens"].(int); got != 2 {
		t.Errorf("completion_tokens = %v, want 2", u["completion_tokens"])
	}
	if got, _ := u["total_tokens"].(int); got != 5 { // 3 + 2 — pins the sum, not a product/difference
		t.Errorf("total_tokens = %v, want 5 (prompt+completion)", u["total_tokens"])
	}
}
