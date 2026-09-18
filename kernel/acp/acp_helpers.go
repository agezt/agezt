// SPDX-License-Identifier: MIT

// acp_helpers.go: newBoundedScanner + scanMessage + flattenPrompt split off from
// acp.go during the Day 211 god-file refactor (#146). Public API unchanged.
package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
)

func newBoundedScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxMessageBytes)
	return sc
}

// scanMessage reads the next non-blank newline-delimited message from sc into v,
// returning io.EOF at a clean end of stream.
func scanMessage(sc *bufio.Scanner, v any) error {
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		return json.Unmarshal(line, v)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// ProtocolVersion is the ACP version this server implements.
const ProtocolVersion = 1

// ChunkKind distinguishes the two streamed content kinds the ACP protocol
// renders differently: the assistant's answer vs. its reasoning / chain of
// thought (M322). A reasoning model's thinking maps to ACP's
// agent_thought_chunk (shown in the editor's "thinking" UI), the answer to
// agent_message_chunk.
type ChunkKind int

const (
	// ChunkMessage is an answer-text delta → agent_message_chunk.
	ChunkMessage ChunkKind = iota
	// ChunkThought is a reasoning delta (M322) → agent_thought_chunk.
	ChunkThought
)

// Runner executes one prompt as an agent run. onChunk is called for each
// streamed delta, tagged with its kind (answer vs. reasoning); the returned
// string is the full final answer (used to emit a single chunk when the
// provider did not stream). All work must pass through the kernel's governed
// path.
type Runner interface {
	Prompt(ctx context.Context, cwd, intent string, onChunk func(kind ChunkKind, text string)) (string, error)
}

// Server speaks ACP over a reader/writer pair.
type Server struct {
	runner Runner
	sc     *bufio.Scanner
	out    io.Writer
	writeM sync.Mutex // serialize notifications vs responses on out

	sessM    sync.Mutex
	sessions map[string]string // sessionId -> cwd
	nextID   int
}

// New builds a Server reading JSON-RPC from in and writing to out.
func flattenPrompt(blocks []contentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Type == "text" || (b.Type == "" && b.Text != "") {
			if out != "" {
				out += "\n"
			}
			out += b.Text
		}
	}
	return out
}
