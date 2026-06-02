// SPDX-License-Identifier: MIT

package plugin

// White-box concurrency test for the death-cause field (M178). deathErr
// is written by the read-loop goroutine (markDead) / Close and read by
// callers; before M178 it was a plain `error` field, so concurrent
// access was a data race the `dead` atomic did not cover. This test
// hammers reads against a concurrent markDead and is meaningful under
// `go test -race`: it must stay clean.

import (
	"errors"
	"sync"
	"testing"
)

func TestDeathErr_ConcurrentReadWrite(t *testing.T) {
	p := &Plugin{
		pending:  make(map[string]chan *Response),
		progress: make(map[string]func(string)),
	}

	var wg sync.WaitGroup
	start := make(chan struct{})

	// Readers: race deathError()/IsAlive() against the writer below.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 2000; j++ {
				_ = p.deathError()
				_ = p.IsAlive()
			}
		}()
	}

	// Writer: mark the plugin dead once, mid-flight.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		p.markDead(errors.New("boom"))
	}()

	close(start)
	wg.Wait()

	if got := p.deathError(); got == nil || got.Error() != "boom" {
		t.Fatalf("deathError() = %v; want boom", got)
	}
	// markDead is idempotent (CompareAndSwap) — a second call must not
	// overwrite the recorded cause.
	p.markDead(errors.New("second"))
	if got := p.deathError(); got.Error() != "boom" {
		t.Errorf("deathError() = %v after second markDead; want boom (idempotent)", got)
	}
}
