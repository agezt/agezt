// SPDX-License-Identifier: MIT
//
// kernel/creds/sigv4 crypto primitives (deriveSigningKey, hmacSHA256, sha256Hex).
// Extracted from sigv4.go during Day 211 god-file refactor (#91).
// Public API unchanged.
package sigv4

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

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
