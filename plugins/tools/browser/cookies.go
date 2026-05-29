// SPDX-License-Identifier: MIT

package browser

import (
	"net/http/cookiejar"
)

// newDefaultJar returns an in-memory cookie jar with stdlib
// defaults — no public-suffix list (M1.mm).
//
// **Why no PSL.** The canonical PSL implementation lives in
// `golang.org/x/net/publicsuffix`, which the lean-deps policy
// excludes. Without PSL, the jar lacks eTLD+1 partitioning — a
// page that sets `Domain=.co.uk` could leak its cookie to other
// `*.co.uk` sites the agent visits.
//
// This is a real risk on a wide-open browser, but the agezt
// browser tool already restricts reads to operator-declared
// AllowedHosts (DECISIONS F2, default-deny). The risk surface is
// "two AllowedHosts on the same eTLD" — narrow, and operators
// declaring such a config are already opting into the trust
// relationship.
//
// Operators who genuinely need PSL-correct partitioning can write
// a thin plugin that wraps a PSL-aware jar; the in-process tool
// stays lean. If the proposal to add PSL to stdlib lands
// (https://github.com/golang/go/issues/61586) we'll switch.
func newDefaultJar() (*cookiejar.Jar, error) {
	return cookiejar.New(nil)
}
