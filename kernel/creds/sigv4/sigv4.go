// SPDX-License-Identifier: MIT

// Package sigv4 implements the AWS Signature Version 4 algorithm.
//
// Extracted from plugins/providers/bedrock during M1.SigV4 so that
// non-Bedrock AWS services (sts:AssumeRole, sso:GetRoleCredentials,
// future S3/DynamoDB/etc.) can sign their own requests without
// duplicating the (subtle and easy-to-get-wrong) canonicalisation.
//
// References:
//
//	https://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
//	https://docs.aws.amazon.com/general/latest/gr/sigv4-create-string-to-sign.html
//	https://docs.aws.amazon.com/general/latest/gr/sigv4-calculate-signature.html
//
// Scope is unchanged from the bedrock-internal version:
//   - POST/GET with a single signed request (no event-stream chunked
//     signing — that's only needed for upload streaming).
//   - Static credentials (AKID + secret + optional STS session token).
//     Higher-level providers (the AWS credential chain in
//     kernel/creds/aws.go) handle discovery and rotation; this
//     package only signs.
//
// Concurrency: SignRequest is pure — it mutates the passed *http.Request
// in place but holds no shared state. Safe to call concurrently with
// different requests.
package sigv4

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
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

// CanonicalQuery serialises the query in AWS canonical order: sort
// by key, then by value, both URL-encoded. Exported so callers that
// build pre-signed URLs (not used yet, but cheap to expose) can reuse
// the same encoding the signer applies.
func CanonicalQuery(q map[string][]string) string {
	if len(q) == 0 {
		return ""
	}
	type kv struct{ k, v string }
	var pairs []kv
	for k, vs := range q {
		for _, v := range vs {
			pairs = append(pairs, kv{AWSURIEncode(k, true), AWSURIEncode(v, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	var parts []string
	for _, p := range pairs {
		parts = append(parts, p.k+"="+p.v)
	}
	return strings.Join(parts, "&")
}

func canonicalHeaders(h http.Header) (canonical, signedHeaders string) {
	const (
		hHost         = "host"
		hXAmzDate     = "x-amz-date"
		hXAmzContent  = "x-amz-content-sha256"
		hContentType  = "content-type"
		hXAmzSecurity = "x-amz-security-token"
	)
	include := []string{hHost, hXAmzDate, hXAmzContent}
	if h.Get("Content-Type") != "" {
		include = append(include, hContentType)
	}
	if h.Get("X-Amz-Security-Token") != "" {
		include = append(include, hXAmzSecurity)
	}
	sort.Strings(include)
	var lines []string
	for _, name := range include {
		val := strings.TrimSpace(h.Get(name))
		val = collapseSpaces(val)
		lines = append(lines, name+":"+val+"\n")
	}
	return strings.Join(lines, ""), strings.Join(include, ";")
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

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// AWSURIEncode is RFC-3986-percent-encode with the AWS twist:
// unreserved chars (A-Z a-z 0-9 - _ . ~) pass through; slash passes
// through when encodeSlash is false (for URI paths), gets %-encoded
// when true (for query values). Exported because callers building
// pre-signed URLs need the same encoding.
func AWSURIEncode(s string, encodeSlash bool) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			sb.WriteByte(c)
		case c == '/' && !encodeSlash:
			sb.WriteByte(c)
		default:
			fmt.Fprintf(&sb, "%%%02X", c)
		}
	}
	return sb.String()
}
