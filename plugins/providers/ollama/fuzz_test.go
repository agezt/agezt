// SPDX-License-Identifier: MIT

package ollama

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
	f.Add([]byte("data: {}\n\n"))
	f.Add([]byte(""))
	f.Add([]byte("not json\n"))
	f.Add([]byte("{\"candidates\":[\n"))
	f.Add([]byte{0x00, 0xff, 0xfe, '\n'})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseStream(bytes.NewReader(data), "fuzz-model", func(agent.Chunk) error { return nil })
	})
}
