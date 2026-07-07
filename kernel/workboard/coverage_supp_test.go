// SPDX-License-Identifier: MIT

package workboard

import (
	"errors"
	"testing"
	"time"
)

func TestParseStatus_Valid(t *testing.T) {
	cases := []string{"triage", "todo", "ready", "running", "blocked", "review", "done", "archived"}
	for _, s := range cases {
		st, err := ParseStatus(s)
		if err != nil {
			t.Errorf("ParseStatus(%q): %v", s, err)
		}
		if Status(s) != st {
			t.Errorf("ParseStatus(%q) = %q, want %q", s, st, s)
		}
	}
}

func TestParseStatus_CaseInsensitive(t *testing.T) {
	st, err := ParseStatus("  DONE  ")
	if err != nil {
		t.Fatalf("ParseStatus('  DONE  '): %v", err)
	}
	if st != StatusDone {
		t.Errorf("ParseStatus('  DONE  ') = %q, want done", st)
	}
}

func TestParseStatus_Empty(t *testing.T) {
	_, err := ParseStatus("")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("ParseStatus('') error = %v, want ErrInvalidStatus", err)
	}
}

func TestParseStatus_Invalid(t *testing.T) {
	_, err := ParseStatus("bogus")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("ParseStatus('bogus') error = %v, want ErrInvalidStatus", err)
	}
}

func TestSetRetryPolicy_ChangesPolicy(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := s.Create(CreateSpec{Title: "test", Status: StatusTodo}, t0)
	if err != nil {
		t.Fatal(err)
	}
	id := task.ID

	rp := &RetryPolicy{MaxAttempts: 3}
	got, err := s.SetRetryPolicy(id, "test", rp, t1)
	if err != nil {
		t.Fatalf("SetRetryPolicy: %v", err)
	}
	if got.RetryPolicy == nil || got.RetryPolicy.MaxAttempts != 3 {
		t.Fatalf("retry policy not set: %+v", got.RetryPolicy)
	}
}

func TestSetRetryPolicy_NotFound(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetRetryPolicy("nonexistent", "test", &RetryPolicy{MaxAttempts: 3}, t0)
	if err == nil {
		t.Fatal("SetRetryPolicy on nonexistent should error")
	}
}

var (
	t0 = fixedTime(1_700_000_000_000)
	t1 = fixedTime(1_700_000_000_001)
)

func fixedTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}
