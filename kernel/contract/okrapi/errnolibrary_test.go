// SPDX-License-Identifier: MIT

package okrapi

import (
	"strings"
	"testing"
)

// The library's ANSWERS both sides name.
//
// The refusal moved here because the host and the module have to be able to
// say the same thing without either naming the other: the host may not name
// the module, and the module may not name the host.

// The refusal is the contract's row.
//
// A daemon with no objective library answers no library, and the answer both
// sides hold is this one.
func TestTheNoLibraryRefusalIsTheContracts(t *testing.T) {
	if ErrNoLibrary == nil {
		t.Fatal("the contract must carry the no-library refusal")
	}
	if !strings.Contains(ErrNoLibrary.Error(), "not available") {
		t.Errorf("the refusal says %q, want it to say the library is not available", ErrNoLibrary.Error())
	}
}

// The refusal is its OWN row.
//
// The board has its own refusal (workboardapi.ErrNoStore) and the library has
// its own. A daemon that holds neither board nor library must be able to say
// WHICH one is missing — one row for both would make the answer a guess.
func TestTheNoLibraryRefusalIsItsOwnRow(t *testing.T) {
	if ErrNoLibrary.Error() == ErrNotFound.Error() {
		t.Error("the no-library refusal must not be the not-found row")
	}
	if !strings.Contains(ErrNoLibrary.Error(), "objective library") {
		t.Errorf("the refusal says %q, want it to name the objective library", ErrNoLibrary.Error())
	}
}
