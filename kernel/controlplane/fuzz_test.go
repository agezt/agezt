// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
)

// FuzzRequestParse hardens the control plane's PRE-AUTH read+parse path — the
// first thing any local client's bytes hit, before the token is checked. It
// drives the custom bounded reader (`readBoundedLine`) and the request unmarshal
// exactly as `handleConn` does. Invariants:
//
//   - readBoundedLine never panics and never returns more than `max` bytes (a
//     no-newline or oversized stream must be bounded, not an OOM).
//   - On a complete line, unmarshalling it into a Request never panics.
func FuzzRequestParse(f *testing.F) {
	f.Add([]byte(`{"cmd":"version","token":"t"}` + "\n"))
	f.Add([]byte(`{"cmd":"run","args":{"tenant":123}}` + "\n")) // wrong-typed arg
	f.Add([]byte("no newline, never terminates"))
	f.Add([]byte("\n"))
	f.Add([]byte{0x00, 0xff, 0xfe, '\n'})

	f.Fuzz(func(t *testing.T, data []byte) {
		const max = 4096
		r := bufio.NewReader(bytes.NewReader(data))
		line, err := readBoundedLine(r, max)
		if len(line) > max {
			t.Fatalf("readBoundedLine returned %d bytes, exceeds max %d", len(line), max)
		}
		if err != nil {
			return // EOF / too-large / read error — handleConn returns here too.
		}
		var req Request
		_ = json.Unmarshal(line, &req) // must not panic on any complete line
	})
}
