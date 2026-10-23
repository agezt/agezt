// SPDX-License-Identifier: MIT

package plugin

import (
	"errors"
	"testing"
)

func TestCheckProtocolVersion_Matches(t *testing.T) {
	if err := checkProtocolVersion(ProtocolVersion); err != nil {
		t.Fatalf("checkProtocolVersion(%d) = %v, want nil", ProtocolVersion, err)
	}
}

func TestCheckProtocolVersion_OmittedDefaultsToV1(t *testing.T) {
	// Plugins written before protocol_version existed omit it (JSON int zero value).
	// They should be treated as v1 for backward compatibility.
	if err := checkProtocolVersion(0); err != nil {
		t.Fatalf("checkProtocolVersion(0) = %v, want nil (back-compat)", err)
	}
}

func TestCheckProtocolVersion_RejectsMismatch(t *testing.T) {
	err := checkProtocolVersion(99)
	if !errors.Is(err, ErrProtocolVersionMismatch) {
		t.Fatalf("checkProtocolVersion(99) = %v, want ErrProtocolVersionMismatch", err)
	}
}

func TestCheckProtocolVersion_RejectsFutureVersion(t *testing.T) {
	err := checkProtocolVersion(2)
	if !errors.Is(err, ErrProtocolVersionMismatch) {
		t.Fatalf("checkProtocolVersion(2) = %v, want ErrProtocolVersionMismatch", err)
	}
}
