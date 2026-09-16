// SPDX-License-Identifier: MIT
//
// kernel/creds/sigv4 SignRequest + buildCanonicalRequest + Creds type.
// Extracted from sigv4.go during Day 211 god-file refactor (#91).
// Public API unchanged.
package sigv4

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Creds carries the static AWS credentials used to sign a single
// request. SessionToken is only present for STS-issued temporary
// credentials (AssumeRole / SSO / IMDS / credential_process).
type Creds struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string // optional
}

// SignRequest mutates req in place, attaching the SigV4 headers
// (Authorization, X-Amz-Date, X-Amz-Content-Sha256, and
// X-Amz-Security-Token when SessionToken is set).
//
// service is the AWS service code that appears in the credential
// scope (e.g. "bedrock", "sts", "awsssoportal", "s3"). It is NOT
// always the same as the hostname — AWS service codes are often
// shorter than their endpoint names (Bedrock-runtime signs as
// "bedrock"; SSO portal signs as "awsssoportal"). Callers MUST
// pass the documented code for their service or AWS will reject
// the request with a signature mismatch.
//
// region is the AWS region (e.g. "us-east-1"). body is the request
// payload; its SHA-256 hex digest is included in both
// X-Amz-Content-Sha256 and the canonical-request hashed-payload
// line. now is injected so tests can pin a known timestamp.
//
// Returns an error only when creds, region, or service are missing.
// The signing itself is deterministic and cannot fail at runtime.
func SignRequest(req *http.Request, service, region string, body []byte, creds Creds, now time.Time) error {
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return errors.New("sigv4: AccessKeyID and SecretAccessKey required")
	}
	if region == "" {
		return errors.New("sigv4: region required")
	}
	if service == "" {
		return errors.New("sigv4: service required")
	}

	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	bodyHash := sha256Hex(body)

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", bodyHash)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}

	canonicalRequest, signedHeaders := buildCanonicalRequest(req, bodyHash)
	credentialScope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(creds.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	auth := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", auth)
	return nil
}
func buildCanonicalRequest(req *http.Request, bodyHash string) (canonical, signedHeaders string) {
	method := strings.ToUpper(req.Method)
	uri := req.URL.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	query := CanonicalQuery(req.URL.Query())
	canonicalHeadersStr, signedHeaders := canonicalHeaders(req.Header)
	canonical = strings.Join([]string{
		method,
		uri,
		query,
		canonicalHeadersStr,
		signedHeaders,
		bodyHash,
	}, "\n")
	return canonical, signedHeaders
}
