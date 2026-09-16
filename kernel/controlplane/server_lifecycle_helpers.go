// SPDX-License-Identifier: MIT
//
// kernel/controlplane Server lifecycle helpers (tokenIsPrimary, maxRequestBytes,
// errRequestTooLarge, readBoundedLine).
// Extracted from server_lifecycle.go during Day 211 god-file refactor (#88).
// Public API unchanged.
package controlplane

import (
	"bufio"
	"crypto/subtle"
	"errors"
)

func (s *Server) tokenIsPrimary(presented string) bool {
	want := s.Token()
	// A blank presented or server token never authorizes (defense in
	// depth, mirroring tenant.Registry.Authorize): subtle.ConstantTimeCompare
	// of two empty strings returns 1, which would let an empty token match
	// an as-yet-unset server token. Emptiness is not token-content, so this
	// short-circuit leaks nothing secret.
	if want == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(want)) == 1
}

// maxRequestBytes bounds a single control-plane request line (M188). The
// request is read before authentication, so any local client reaching
// the loopback port can stream bytes here; 16 MiB is far above any
// legitimate command (even a large inline run prompt) while bounding a
// pre-auth memory-exhaustion DoS.
const maxRequestBytes = 16 << 20

// errRequestTooLarge is returned when a request line exceeds maxRequestBytes.
var errRequestTooLarge = errors.New("controlplane: request exceeds max size")

// readBoundedLine reads one newline-delimited line from r, bounding the
// total to max bytes (M188). It reads in buffer-sized ReadSlice chunks
// (which return bufio.ErrBufferFull for a line longer than the reader's
// buffer), copying each out before the next read so the returned slice is
// stable, and returns errRequestTooLarge once the accumulated line would
// exceed max — instead of allocating without bound. A trailing chunk with
// io.EOF (stream ended mid-line) is returned with that error.
func readBoundedLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > max {
			return nil, errRequestTooLarge
		}
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return buf, err
	}
}
