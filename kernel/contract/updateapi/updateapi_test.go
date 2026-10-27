// SPDX-License-Identifier: MIT

package updateapi_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/updateapi"
)

// TestTheChecksumMismatchMessageSurvivesAShortHash: the wanted hash comes
// from a manifest, and a manifest may carry a short one.
//
// The message TRUNCATES the two hexes; it does not slice them. The old
// `e.Want[:8]` panicked while reporting a failed checksum — a crash in the
// error path, which is where a daemon is already unhappy.
func TestTheChecksumMismatchMessageSurvivesAShortHash(t *testing.T) {
	mismatch := &updateapi.ErrChecksumMismatch{Have: "deadbeefcafef00d", Want: "abc"}
	message := mismatch.Error()
	if !strings.Contains(message, "deadbeef") || !strings.Contains(message, "want=abc") {
		t.Fatalf("EXPECTED:both hashes in the message ACTUAL:%q", message)
	}
	if strings.Contains(message, "cafef00d") {
		t.Fatalf("EXPECTED:the hash truncated ACTUAL:%q", message)
	}
}

// TestTheChecksumMismatchMessageTruncatesLongHashes: a full digest is shown
// by its first eight characters.
//
// The assertion is about what is NOT there: "have=01234567" is a prefix of
// "have=0123456789abcdef", so a message that shows the whole hash would
// still contain it.
func TestTheChecksumMismatchMessageTruncatesLongHashes(t *testing.T) {
	mismatch := &updateapi.ErrChecksumMismatch{Have: "0123456789abcdef", Want: "fedcba9876543210"}
	message := mismatch.Error()
	if !strings.Contains(message, "have=01234567") || !strings.Contains(message, "want=fedcba98") {
		t.Fatalf("EXPECTED:both hashes truncated to eight characters ACTUAL:%q", message)
	}
	if strings.Contains(message, "0123456789abcdef") || strings.Contains(message, "fedcba9876543210") {
		t.Fatalf("EXPECTED:the whole hash NOT in the message ACTUAL:%q", message)
	}
}

// TestTheSignatureInvalidMessageCarriesItsReason: the reason is the whole
// message of that error.
func TestTheSignatureInvalidMessageCarriesItsReason(t *testing.T) {
	invalid := &updateapi.ErrSignatureInvalid{Reason: "signature does not match"}
	if !strings.Contains(invalid.Error(), "signature does not match") {
		t.Fatalf("EXPECTED:the reason in the message ACTUAL:%q", invalid.Error())
	}
}
