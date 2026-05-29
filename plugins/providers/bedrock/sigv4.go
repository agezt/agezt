// SPDX-License-Identifier: MIT

package bedrock

// SigV4 shim — the algorithm itself lives in kernel/creds/sigv4
// after the M1.SigV4 extraction. This file preserves the in-package
// names (SigV4Creds, signRequest, the helpers) that the existing
// tests and Provider wiring use, and forwards to the kernel
// implementation with service="bedrock".
//
// Keeping this shim (rather than rewriting callers) means:
//   - The bedrock-side tests in sigv4_test.go that reach into
//     package internals (canonicalQuery, awsURIEncode, sha256Hex,
//     collapseSpaces) keep working untouched.
//   - Provider.SetSigV4Creds(*SigV4Creds) is unchanged, so the
//     operator-facing API for Bedrock didn't shift sideways during
//     the extraction.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/ersinkoc/agezt/kernel/creds/sigv4"
)

// SigV4Creds is the bedrock-package alias for sigv4.Creds.
// Operators continue to set this via Provider.SetSigV4Creds; the
// underlying type is shared so creds discovered by
// kernel/creds.AWSDefaultChain (which returns sigv4.Creds) can be
// passed straight through.
type SigV4Creds = sigv4.Creds

// sigV4Service is the AWS service code Bedrock signs against.
// (Yes — "bedrock", not "bedrock-runtime" despite the hostname.
// AWS service codes are often shorter than their endpoint names.)
const sigV4Service = "bedrock"

func signRequest(req *http.Request, region string, body []byte, creds SigV4Creds, now time.Time) error {
	return sigv4.SignRequest(req, sigV4Service, region, body, creds, now)
}

// The helpers below are kept as thin forwards so the existing
// internal tests (canonical-query ordering, URI encoding,
// whitespace collapsing, SHA-256-hex helper) keep working without
// being rewritten to import the kernel package.

func canonicalQuery(q map[string][]string) string { return sigv4.CanonicalQuery(q) }
func awsURIEncode(s string, encodeSlash bool) string {
	return sigv4.AWSURIEncode(s, encodeSlash)
}

// sha256Hex and collapseSpaces are kept here (not forwarded to the
// kernel package) because the existing internal tests in
// sigv4_test.go reach them by unqualified name. Re-implementing
// the two trivial helpers in the shim is cheaper than restructuring
// the tests just to move two lines.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func collapseSpaces(s string) string {
	if !strings.Contains(s, "  ") {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if !prevSpace {
				sb.WriteRune(r)
			}
			prevSpace = true
		} else {
			sb.WriteRune(r)
			prevSpace = false
		}
	}
	return sb.String()
}
