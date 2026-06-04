// SPDX-License-Identifier: MIT

package skill

import (
	"fmt"
	"strings"
)

// ShadowTestMinBodyChars is the minimum body length a draft must have to pass the
// shadow-test — short enough to admit a one-liner skill, long enough to reject an
// empty/stub body that couldn't carry real instructions.
const ShadowTestMinBodyChars = 16

// ShadowTest is the deterministic gate on the draft→shadow transition
// (SPEC-05 §5.2: "draft —(shadow-test passes)→ shadow"). v1 is a structural
// validation — the skill must be well-formed and *retrievable* — not the richer
// execution-comparison that gates shadow→active (run alongside, promote only if
// it would have helped); that needs a shadow-execution harness and lands later.
//
// A draft fails the test when it could never function in production:
//   - an empty or trivially short body (no real instructions to inject), or
//   - no description AND no triggers, so retrieval (which scores on
//     description+triggers) could never surface it.
//
// It is pure so it can gate auto-staging and be unit-tested in isolation; the
// returned reason is journaled on a rejection.
func ShadowTest(sk Skill) (ok bool, reason string) {
	body := strings.TrimSpace(sk.Body)
	if body == "" {
		return false, "empty body"
	}
	if len([]rune(body)) < ShadowTestMinBodyChars {
		return false, fmt.Sprintf("body too short (<%d chars) to be a substantive skill", ShadowTestMinBodyChars)
	}
	if strings.TrimSpace(sk.Description) == "" && len(sk.Triggers) == 0 {
		return false, "no description or triggers — the skill could never be retrieved"
	}
	return true, ""
}
