// SPDX-License-Identifier: MIT

package openai

import (
	"bytes"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// FuzzParseStream hardens the streaming response parser against a malformed,
// truncated, or hostile upstream (a MITM/buggy proxy in front of the provider).
// Invariant: parseStream never panics or hangs on arbitrary bytes — a garbage
// stream must yield a clean error, not crash the agent loop. (The bounded scanner
// and per-frame handling were code-reviewed in M431; this is the fuzz backstop.)
func FuzzParseStream(f *testing.F) {
	f.Add([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"))
	f.Add([]byte(""))
	f.Add([]byte("data: not json\n\n"))
	f.Add([]byte("event: x\ndata: {\n\n"))
	f.Add([]byte{0x00, 0xff, 0xfe, '\n'})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseStream(bytes.NewReader(data), func(agent.Chunk) error { return nil })
	})
}
