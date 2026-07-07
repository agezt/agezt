// SPDX-License-Identifier: MIT

package agentgw

import (
	"testing"

	"github.com/agezt/agezt/kernel/edict"
)

func TestHasAnyCap_MatchFirst(t *testing.T) {
	c := &TokenClaims{Caps: []string{"memory.read", "log.write"}}
	if !c.HasAnyCap(edict.Capability("memory.read"), edict.Capability("log.read")) {
		t.Error("HasAnyCap should match first cap")
	}
}

func TestHasAnyCap_MatchSecond(t *testing.T) {
	c := &TokenClaims{Caps: []string{"memory.read", "log.write"}}
	if !c.HasAnyCap(edict.Capability("log.read"), edict.Capability("log.write")) {
		t.Error("HasAnyCap should match second cap")
	}
}

func TestHasAnyCap_NoMatch(t *testing.T) {
	c := &TokenClaims{Caps: []string{"memory.read"}}
	if c.HasAnyCap(edict.Capability("log.read"), edict.Capability("agent.list")) {
		t.Error("HasAnyCap should return false when no caps match")
	}
}

func TestHasAnyCap_Empty(t *testing.T) {
	c := &TokenClaims{}
	if c.HasAnyCap() {
		t.Error("HasAnyCap with no args should return false")
	}
}

func TestLastSeen_ReturnsRecentTime(t *testing.T) {
	rl := NewRateLimit(10, 5)
	rl.Allow()
	ls := rl.LastSeen()
	if ls <= 0 {
		t.Errorf("LastSeen() = %d, want > 0", ls)
	}
}

func TestNewTokenManager_ShortSecret(t *testing.T) {
	tm := NewTokenManager([]byte("short"))
	if tm == nil {
		t.Fatal("NewTokenManager should not return nil for short secret")
	}
	if len(tm.secret) < 32 {
		t.Errorf("secret length = %d, want at least 32", len(tm.secret))
	}
}
