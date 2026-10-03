// SPDX-License-Identifier: MIT

package builtintools

import (
	"bytes"
	"testing"

	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/tools/browser"
)

// AGEZT_BROWSER_COOKIES=1 is the documented opt-in (M1.mm) for a session cookie
// jar on browser.read, and it is listed in the Config Center. A refactor dropped
// the wiring — the builder carried a comment pointing at a different tool — so
// the setting did nothing and browser.read never kept a cookie.
func TestBrowserRead_CookiesOptIn(t *testing.T) {
	build := func(env map[string]string) *browser.Tool {
		t.Helper()
		built, err := buildBrowserRead(toolreg.BuildDeps{Stderr: &bytes.Buffer{}, Get: func(k string) string { return env[k] }})
		if err != nil {
			t.Fatalf("buildBrowserRead: %v", err)
		}
		br, ok := built.Tool.(*browser.Tool)
		if !ok {
			t.Fatalf("built %T, want *browser.Tool", built.Tool)
		}
		return br
	}
	if build(map[string]string{}).Cookies != nil {
		t.Error("cookie jar attached without the opt-in")
	}
	if build(map[string]string{"AGEZT_BROWSER_COOKIES": "1"}).Cookies == nil {
		t.Error("AGEZT_BROWSER_COOKIES=1 did not attach a cookie jar to browser.read")
	}
}
