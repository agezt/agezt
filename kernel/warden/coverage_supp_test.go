// SPDX-License-Identifier: MIT

package warden

import "testing"

func TestDowngradeReason_KnownProfiles(t *testing.T) {
	if r := downgradeReason(ProfileNamespace); r == "" {
		t.Error("downgradeReason(Namespace) should not be empty")
	}
	if r := downgradeReason(ProfileContainer); r == "" {
		t.Error("downgradeReason(Container) should not be empty")
	}
	if r := downgradeReason(ProfileMicroVM); r == "" {
		t.Error("downgradeReason(MicroVM) should not be empty")
	}
}

func TestDowngradeReason_Unknown(t *testing.T) {
	r := downgradeReason(Profile("bogus"))
	if r != "unknown profile" {
		t.Errorf("downgradeReason(bogus) = %q, want 'unknown profile'", r)
	}
}

func TestDowngradeReason_Empty(t *testing.T) {
	r := downgradeReason(Profile(""))
	if r == "" {
		t.Error("downgradeReason('') should not be empty (falls to unknown profile)")
	}
}
