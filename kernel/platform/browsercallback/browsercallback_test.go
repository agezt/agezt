// SPDX-License-Identifier: MIT

package browsercallback_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/platform/browsercallback"
)

func TestRenderPreservesSuccessFailureAndHTMLText(t *testing.T) {
	for _, success := range []bool{true, false} {
		w := httptest.NewRecorder()
		browsercallback.Render(w, success, `<script>"fixture" & text</script>`)
		body := w.Body.String()
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(body, "window.close()},1500)") {
			t.Fatalf("page framing changed: %d %v %s", w.Code, w.Header(), body)
		}
		if success {
			if !strings.Contains(body, "Signed in ✓") || !strings.Contains(body, "return to the console") || strings.Contains(body, "fixture") {
				t.Fatalf("success page changed: %s", body)
			}
		} else if !strings.Contains(body, "Sign-in failed") || !strings.Contains(body, "&lt;script&gt;&quot;fixture&quot; &amp; text&lt;/script&gt;") || strings.Contains(body, `<script>"fixture"`) {
			t.Fatalf("failure text escaped incorrectly: %s", body)
		}
	}
}

func TestHandlePreservesQueryContextAndClosesAfterRendering(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "parent"))
	cancel()
	r := httptest.NewRequest("GET", "http://callback.invalid/?code=first&code=second&state=owned%20state&error=denied%2Breason", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	closed := make(chan string, 1)
	browsercallback.Handle(w, r, func(actual context.Context, code, state, denial string) (bool, string, bool) {
		if actual != ctx || actual.Value(key{}) != "parent" || !errors.Is(actual.Err(), context.Canceled) || code != "first" || state != "owned state" || denial != "denied+reason" {
			t.Fatalf("callback projection changed: %v %q %q %q", actual, code, state, denial)
		}
		return false, "fixture failed", true
	}, func() { closed <- w.Body.String() })
	select {
	case body := <-closed:
		if !strings.Contains(body, "fixture failed") || !strings.Contains(body, "Sign-in failed") {
			t.Fatalf("close started before rendering: %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("requested listener close was not dispatched")
	}
}

func TestHandleKeepsListenerWhenCloseNotRequested(t *testing.T) {
	w := httptest.NewRecorder()
	called := make(chan struct{}, 1)
	browsercallback.Handle(w, httptest.NewRequest("GET", "http://callback.invalid/", nil), func(_ context.Context, code, state, denial string) (bool, string, bool) {
		if code != "" || state != "" || denial != "" {
			t.Fatal("missing query fields changed")
		}
		return false, "invalid state", false
	}, func() { called <- struct{}{} })
	select {
	case <-called:
		t.Fatal("invalid nonterminal callback closed listener")
	case <-time.After(10 * time.Millisecond):
	}
	if !strings.Contains(w.Body.String(), "invalid state") {
		t.Fatal("callback message lost")
	}
}
