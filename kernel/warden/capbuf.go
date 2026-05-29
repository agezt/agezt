// SPDX-License-Identifier: MIT

package warden

// capBuffer is an io.Writer that retains at most max bytes total,
// tail-truncating older data so the consumer sees the *most-recent*
// output. We tail-truncate (not head-truncate) because for diagnosing
// a failing command the last few lines of stderr matter most.
type capBuffer struct {
	max       int
	buf       []byte
	truncated bool
}

func newCapBuffer(max int) *capBuffer {
	if max <= 0 {
		max = DefaultMaxOutputBytes
	}
	return &capBuffer{max: max, buf: make([]byte, 0, min(max, 4096))}
}

// Write appends p, tail-truncating to keep at most max bytes.
func (c *capBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	// Fast path: still under cap.
	if len(c.buf)+n <= c.max {
		c.buf = append(c.buf, p...)
		return n, nil
	}
	c.truncated = true
	combined := len(c.buf) + n
	keep := c.max
	drop := combined - keep
	switch {
	case drop >= len(c.buf):
		// Drop the entire existing buffer; keep only the tail of p.
		c.buf = c.buf[:0]
		off := max(n-keep, 0)
		c.buf = append(c.buf, p[off:]...)
	default:
		// Drop the head of c.buf, then append all of p.
		c.buf = append(c.buf[:0], c.buf[drop:]...)
		c.buf = append(c.buf, p...)
	}
	return n, nil
}

// Bytes returns the retained tail (caller must not mutate).
func (c *capBuffer) Bytes() []byte { return c.buf }

// Truncated reports whether Write has ever dropped data.
func (c *capBuffer) Truncated() bool { return c.truncated }
