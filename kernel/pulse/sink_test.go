// SPDX-License-Identifier: MIT

package pulse

import (
	"errors"
	"testing"
)

func TestSinkFunc(t *testing.T) {
	var got Brief
	s := SinkFunc(func(b Brief) error { got = b; return nil })
	_ = s.Deliver(Brief{Title: "hi"})
	if got.Title != "hi" {
		t.Fatalf("SinkFunc did not forward the brief: %+v", got)
	}
}

func TestMultiSinkFansOutAndContinuesOnError(t *testing.T) {
	count := 0
	failing := SinkFunc(func(Brief) error { count++; return errors.New("boom") })
	ok := SinkFunc(func(Brief) error { count++; return nil })
	m := MultiSink{failing, nil, ok}

	err := m.Deliver(Brief{Title: "x"})
	if err == nil {
		t.Fatal("MultiSink should surface the first error")
	}
	if count != 2 {
		t.Fatalf("both non-nil sinks should run despite an error; ran %d", count)
	}
}
