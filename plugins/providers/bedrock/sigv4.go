// SPDX-License-Identifier: MIT

package bedrock

// SigV4 shim — the algorithm itself lives in kernel/creds/sigv4 after the
// M1.SigV4 extraction. This file preserves the in-package names used by
// Provider wiring and forwards to the kernel implementation with service="bedrock".

import (
	"net/http"
	"time"

	"github.com/agezt/agezt/kernel/creds/sigv4"
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
