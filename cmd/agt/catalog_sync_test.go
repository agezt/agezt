// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdCatalogSync_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCatalogSync([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "catalog sync") || !strings.Contains(out.String(), "--local") {
		t.Errorf("help missing usage/--local; got %q", out.String())
	}
}

func TestCmdCatalogSync_BadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCatalogSync([]string{"--nope"}, &out, &errOut); code != 2 {
		t.Errorf("unknown flag should be exit 2, got %d", code)
	}
}

func TestCmdCatalogSync_TooManyPositionals(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCatalogSync([]string{"https://a", "https://b"}, &out, &errOut); code != 2 {
		t.Errorf("two URLs should be exit 2, got %d", code)
	}
}

func TestRenderSyncResultText(t *testing.T) {
	var out bytes.Buffer
	res := map[string]any{
		"url": "https://models.dev/api.json", "provider_count": float64(137),
		"model_count": float64(4979), "duration_ms": float64(165),
	}
	if code := renderSyncResult(res, "local (offline)", false, &out); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	s := out.String()
	for _, want := range []string{"local (offline)", "137", "4979", "165ms"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q in:\n%s", want, s)
		}
	}
}
