// SPDX-License-Identifier: MIT

package agentgw

// Provenance: symbols of this package that have NO production caller — the
//             only consumers are this package's own tests.
//
//             They live in a _test.go file on purpose. The dead-code gate
//             (tools/deadcodecheck) runs the analyzer WITHOUT -test, so a
//             symbol whose only consumer is a test is unreachable in the
//             binaries — and the gate's own rule says such a symbol belongs
//             here rather than in an allowlist: "move it into a _test.go in
//             its own package instead, which states what it is and keeps the
//             gate strict."
//
//             The HTTP mint path (POST /v1/token/create) is the PRODUCTION
//             implementation of the subprocess-token rule, and it is stricter
//             than the copy below in one place: it REFUSES a capability
//             escalation with 403 where this copy silently drops the extra
//             capability. The copy is kept as the test baseline the HTTP path
//             is compared against (see parent_token_id_test.go and
//             security_test.go) — the two must agree on the parent token id
//             and on the capability subset.

import (
	"fmt"
	"strings"
	"time"
)

// LogSync records an entry and flushes immediately.
func (a *AuditLogger) LogSync(entry AuditEntry) {
	a.mu.Lock()
	a.buf = append(a.buf, entry)
	entries := a.buf
	// Hand the backing array off to writeEntries and start a fresh buffer.
	// Reusing it via a.buf[:0] would let a concurrent Log append into the same
	// array elements that writeEntries is reading unlocked — a data race.
	a.buf = make([]AuditEntry, 0, 64)
	a.mu.Unlock()

	a.writeEntries(entries)
}

// CheckAny validates that the token has at least one of the required capabilities.
func (cc *CapabilityChecker) CheckAny(claims *TokenClaims, required ...AgentCapability) error {
	for _, cap := range required {
		if err := cc.Check(claims, cap); err == nil {
			return nil
		}
	}
	return fmt.Errorf("agentgw: none of the required capabilities %v granted", required)
}

// CheckAll validates that the token has all of the required capabilities.
func (cc *CapabilityChecker) CheckAll(claims *TokenClaims, required ...AgentCapability) error {
	for _, cap := range required {
		if err := cc.Check(claims, cap); err != nil {
			return err
		}
	}
	return nil
}

// CapsIntersect returns the capabilities present in BOTH want and parent,
// preserving want's order and dropping anything the parent does not grant.
func CapsIntersect(want, parent []string) []string {
	have := make(map[string]bool, len(parent))
	for _, c := range parent {
		have[strings.ToLower(strings.TrimSpace(c))] = true
	}
	out := make([]string, 0, len(want))
	for _, c := range want {
		if have[strings.ToLower(strings.TrimSpace(c))] {
			out = append(out, c)
		}
	}
	return out
}

// CreateSubprocessToken creates a scoped token for a subprocess.
// It inherits capabilities from the parent but can have a subset.
func (tm *TokenManager) CreateSubprocessToken(parent *TokenClaims, subID string, caps []string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = 10 * time.Minute // default 10 minutes for subprocess
	}

	// Enforce capability subset: a child can never hold a capability the parent
	// lacks. Drop any requested cap the parent doesn't have rather than granting
	// an escalation.
	granted := CapsIntersect(caps, parent.Caps)

	// A subprocess token must never outlive its parent.
	exp := time.Now().Add(expiry)
	if !parent.ExpiresAt.IsZero() && exp.After(parent.ExpiresAt) {
		exp = parent.ExpiresAt
	}

	// Halve the burst — but never below 1: a parent capped at burst 1 would
	// halve to 0, and CreateToken re-defaults a zero MaxBurst to 10, handing
	// the child MORE burst than the parent holds. A subprocess must never
	// exceed its parent's limits, the same way it must never outlive them.
	// (The HTTP mint path CLAMPS the burst to the parent's instead — it does
	// not halve it — and refuses a capability escalation with 403 where this
	// copy drops the extra capability. The HTTP path is the production one.)
	burst := parent.MaxBurst / 2
	if burst < 1 {
		burst = 1
	}

	claims := &TokenClaims{
		RunID:         parent.RunID,
		Caps:          granted,
		MaxRate:       parent.MaxRate,
		MaxBurst:      burst,
		ExpiresAt:     exp,
		ParentTokenID: parent.TokenID,
		SubprocessID:  subID,
	}

	return tm.CreateToken(claims)
}
