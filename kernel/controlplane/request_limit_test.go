// SPDX-License-Identifier: MIT

package controlplane

// White-box test for the bounded control-plane request reader (M188).

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadBoundedLine_Request(t *testing.T) {
	// Under cap: a normal terminated line round-trips.
	r := bufio.NewReader(strings.NewReader("hello\n"))
	got, err := readBoundedLine(r, 1024)
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("under cap: got %q err %v", got, err)
	}

	// Overflow: an unterminated flood larger than the cap is rejected
	// (rather than allocating without bound).
	big := bufio.NewReader(bytes.NewReader(bytes.Repeat([]byte("x"), 5000)))
	if _, err := readBoundedLine(big, 1000); !errors.Is(err, errRequestTooLarge) {
		t.Errorf("overflow err = %v want errRequestTooLarge", err)
	}

	// EOF mid-line returns the partial bytes with io.EOF.
	r2 := bufio.NewReader(strings.NewReader("partial"))
	if g, err := readBoundedLine(r2, 1024); err != io.EOF || string(g) != "partial" {
		t.Errorf("eof-mid-line: got %q err %v", g, err)
	}
}
