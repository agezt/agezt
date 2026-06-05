// SPDX-License-Identifier: MIT

package anthropic

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// malformedFrameStream has a broken content_block_delta BETWEEN two valid text
// deltas. A tolerant parser skips the bad frame and keeps the text on both sides;
// a strict one would abort and discard everything already streamed.
const malformedFrameStream = `event: message_start
data: {"type":"message_start","message":{"model":"claude-3-5-haiku-20241022","usage":{"input_tokens":5,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"pong"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{BROKEN JSON HERE

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_stop
data: {"type":"message_stop"}

`

// TestParseStream_ToleratesMalformedFrame: a single malformed structural frame
// mid-stream must be skipped, not abort the whole stream — matching the other
// providers and this parser's own EOF handling. The text before AND after the bad
// frame is preserved (M451).
func TestParseStream_ToleratesMalformedFrame(t *testing.T) {
	resp, err := parseStream(strings.NewReader(malformedFrameStream), func(agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("a malformed mid-stream frame must not abort the stream: %v", err)
	}
	if resp.Message.Content != "pong!" {
		t.Errorf("content = %q, want %q (text on both sides of the bad frame preserved)", resp.Message.Content, "pong!")
	}
}
