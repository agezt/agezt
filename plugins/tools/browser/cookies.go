// SPDX-License-Identifier: MIT

package browser

import (
	"net/http/cookiejar"

	"golang.org/x/net/publicsuffix"
)

// newDefaultJar returns an in-memory cookie jar with PSL-aware
// eTLD+1 partitioning, so a page that sets `Domain=.co.uk` cannot
// leak its cookie to other `*.co.uk` sites the agent visits.
// `publicsuffix.List` is the stdlib-adopted public suffix
// implementation from `golang.org/x/net` (already a transitive dep
// in the project via `emersion/go-message` → `golang.org/x/text` →
// `golang.org/x/tools`). The jar uses the default implementation,
// which is nil-safe and uses PSL when available.
func newDefaultJar() (*cookiejar.Jar, error) {
	return cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
}
