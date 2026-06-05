// SPDX-License-Identifier: MIT

package bedrock

import (
	"bytes"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// FuzzParseEventStream hardens the AWS event-stream BINARY parser — the highest
// panic-risk parser in the tree, since it reads attacker-influenceable
// length/offset fields (totalLen, headersLen, per-header valueLen) with
// binary.BigEndian and slices buffers by them. A malformed/truncated/hostile
// frame (a MITM/buggy proxy) must yield a clean error, never an
// out-of-bounds panic, a huge allocation, or a hang.
func FuzzParseEventStream(f *testing.F) {
	f.Add([]byte{})
	// totalLen=16, headersLen=0 (smallest accepted frame shape) + 4 CRC bytes.
	f.Add([]byte{0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte("not an event stream"))
	f.Add(make([]byte, 32))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00}) // huge totalLen

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseEventStream(bytes.NewReader(data), "fuzz-model", func(agent.Chunk) error { return nil })
	})
}
