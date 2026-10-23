// SPDX-License-Identifier: MIT

package plugin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/modules/extensions/plugin"
)

// TestSpawn_AllowlistAcceptsListedTools: when the allowlist
// covers every advertised tool, Spawn succeeds normally.
func TestSpawn_AllowlistAcceptsListedTools(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:         bin,
		AllowedTools: []string{"echo", "fail", "slowwork", "callhost"}, // all four advertised tools
	})
	if err != nil {
		t.Fatalf("Spawn with covering allowlist: %v", err)
	}
	defer p.Close()
	tools := p.Tools("")
	if len(tools) != 4 {
		t.Errorf("expected 4 tools registered, got %d", len(tools))
	}
}

// TestSpawn_AllowlistRejectsExtraTool: when the allowlist omits
// one of the advertised tools, Spawn fails BEFORE registering
// any tools — operators want a hard fail on capability drift,
// not a partial allow that silently registers the listed tools.
func TestSpawn_AllowlistRejectsExtraTool(t *testing.T) {
	bin := buildEchoPlugin(t)
	_, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:         bin,
		AllowedTools: []string{"echo"}, // omits "fail"
	})
	if err == nil {
		t.Fatal("Spawn with under-covering allowlist: expected error")
	}
	if !errors.Is(err, plugin.ErrToolAllowlistMismatch) {
		t.Errorf("err is not ErrToolAllowlistMismatch: %v", err)
	}
	if !strings.Contains(err.Error(), "fail") {
		t.Errorf("error doesn't mention the offending tool name: %v", err)
	}
}

// TestSpawn_EmptyAllowlistMeansNoCheck: an empty AllowedTools
// disables verification, matching the opt-in security pattern
// PinnedHash uses.
func TestSpawn_EmptyAllowlistMeansNoCheck(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:         bin,
		AllowedTools: nil, // explicitly empty
	})
	if err != nil {
		t.Fatalf("Spawn with empty allowlist: %v", err)
	}
	defer p.Close()
	if len(p.Tools("")) != 4 {
		t.Error("expected 4 tools without an allowlist")
	}
}
