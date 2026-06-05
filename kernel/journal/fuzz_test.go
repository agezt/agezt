// SPDX-License-Identifier: MIT

package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// FuzzJournalOpen hardens the journal reopen path against a CORRUPT on-disk
// segment — a half-written line from a crash, bit-rot, or tampering. This is the
// custom-parser surface (the M417 torn-tail truncation `lastCompleteOffset`, the
// `scanCompleteLines` split func, segment scanning, and the hash-chain `Verify`).
// A corrupt journal must never crash or hang the daemon on startup: Open may
// reject it with an error or accept it with the torn tail truncated, but every
// path must terminate without panicking.
func FuzzJournalOpen(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("not json at all\n"))
	f.Add([]byte(`{"seq":1,"hash":"deadbeef","kind":"x"}` + "\n"))
	f.Add([]byte(`{"seq":1}` + "\n" + `{"seq":2,"hash":`)) // torn tail mid-line
	f.Add([]byte("\n\n\n"))
	f.Add([]byte{0x00, 0x01, 0xff, 0xfe, '\n'})

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		// Write the bytes as the first segment Open will scan (00000001.jsonl).
		name := fmt.Sprintf("%0*d%s", segmentDigits, 1, segmentExt)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Skip()
		}

		// Open may return an error for a corrupt segment — that's correct handling,
		// not a bug. The invariant is only that nothing panics or hangs.
		j, err := Open(dir, Options{})
		if err != nil {
			return
		}
		defer j.Close()

		_ = j.Range(func(*event.Event) error { return nil })
		_, _ = j.Tail(8)
		_ = j.Verify()
		_, _ = j.Head()
	})
}
