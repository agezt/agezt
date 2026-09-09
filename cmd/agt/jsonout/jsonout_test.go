// SPDX-License-Identifier: MIT

package jsonout_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func TestWrite_PrettyAndValid(t *testing.T) {
	var buf bytes.Buffer
	code := jsonout.Write(&buf, map[string]any{"a": 1, "b": []int{1, 2}})
	if code != 0 {
		t.Errorf("Write returned %d, want 0", code)
	}
	out := buf.String()
	// The encoder always emits a trailing newline; 2-space
	// indent is the documented shape.
	if !strings.HasPrefix(out, "{\n  \"a\": 1,\n  \"b\": [\n    1,\n    2\n  ]\n}\n") {
		t.Errorf("unexpected shape: %q", out)
	}
}

func TestWrite_NilWriterStillReturnsZero(t *testing.T) {
	// Even when the write fails (closed pipe, etc.), the
	// function still returns 0 so the caller's signature is
	// stable. This is the documented contract.
	if code := jsonout.Write(&brokenWriter{}, map[string]int{"x": 1}); code != 0 {
		t.Errorf("Write on broken writer returned %d, want 0", code)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write(p []byte) (int, error) { return 0, errBrokenPipe }

var errBrokenPipe = &writeErr{}

type writeErr struct{}

func (*writeErr) Error() string { return "broken pipe (test)" }
