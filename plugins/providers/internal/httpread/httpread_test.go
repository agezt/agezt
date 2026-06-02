// SPDX-License-Identifier: MIT

package httpread

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestAll_UnderCap(t *testing.T) {
	body := strings.NewReader("hello world")
	got, err := All(body, 1024)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("got %q want %q", got, "hello world")
	}
}

func TestAll_AtCap(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1000)
	got, err := All(bytes.NewReader(data), 1000)
	if err != nil {
		t.Fatalf("at cap should not error: %v", err)
	}
	if len(got) != 1000 {
		t.Errorf("len = %d want 1000", len(got))
	}
}

func TestAll_OverCapRejected(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 5000)
	got, err := All(bytes.NewReader(data), 1000)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("err = %v want ErrResponseTooLarge", err)
	}
	// Returns the first max bytes (so callers can still surface a snippet).
	if len(got) != 1000 {
		t.Errorf("truncated len = %d want 1000", len(got))
	}
}

func TestAll_ZeroMaxUsesDefault(t *testing.T) {
	got, err := All(strings.NewReader("ok"), 0)
	if err != nil || string(got) != "ok" {
		t.Errorf("zero max: got %q err %v", got, err)
	}
}

// errReader returns a read error after some bytes.
type errReader struct{ msg string }

func (e errReader) Read(p []byte) (int, error) { return 0, errors.New(e.msg) }

func TestAll_ReadErrorPassedThrough(t *testing.T) {
	_, err := All(errReader{"boom"}, 1024)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("read error not surfaced: %v", err)
	}
}
