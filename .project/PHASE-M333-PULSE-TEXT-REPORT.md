# M333 — `agt pulse --text`: live content in the operator's tail

## Why
The reasoning workstream (M317–M325) made a reasoning model's chain of thought
reach editors (ACP `agent_thought_chunk`) and API clients (`reasoning_content`),
but the operator's own live view — `agt pulse` — still showed only a one-line
`kind/subject/actor` entry per event. The M317 code comment even claimed reasoning
was "visible live (agt pulse)", which wasn't fully true: the *event* appeared, but
its *text* didn't. This was the last consumer that captured-but-didn't-show
reasoning. (The earlier worry that per-delta lines would be "too noisy" is resolved
by making it opt-in: an operator who asks for `--text` wants the content.)

## What
- **`cmd/agt/pulse.go`**:
  - New `--text` / `-t` flag (off by default). When set, the human renderer
    appends `▸ <excerpt>` to each event line.
  - `eventTextExcerpt(ev)`: pulls the `text` payload field (the streamed answer
    tokens *and* a reasoning model's `llm.reasoning` deltas both carry it),
    collapses all whitespace runs to single spaces (so the tail stays one line per
    event), and truncates to 160 chars + `…`. Empty for events without a `text`
    field — so non-content events (and the durable `llm.response`, which carries
    only `reasoning_chars`) are unaffected.
  - `renderEventHuman` gained a `showText bool` parameter; the default
    structured line is byte-for-byte unchanged when it's false.
  - Help text + usage updated.

## Verification
- **`cmd/agt/pulse_text_test.go`** (new, white-box): `--text` off leaves the line
  free of payload text (and still shows the kind); `--text` on appends the
  reasoning excerpt. `eventTextExcerpt` cases: plain text, newline/tab collapsing,
  no-`text`-field → empty, no-payload → empty, and long-text truncation stays
  bounded.
- **Real CLI smoke**: built `agt`, ran `agt pulse --help` — the `--text` flag and
  its description appear in the usage; exit 0.
- Full suite **2032** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Opt-in: default `agt pulse` output (and any scripts parsing it) are unchanged.
- A live daemon `agt pulse --text` showing streamed text needs a streaming
  provider; the offline mock emits no `llm.token`/`llm.reasoning` events, so the
  render logic is proven by the unit tests against real event payloads rather than
  a daemon curl (the tail also blocks until Ctrl+C, so it isn't a clean smoke
  target).
- With this, reasoning is visible across **every** surface: editors (ACP, M322),
  OpenAI-compatible API (M323/M324), and the operator's live tail (M333).
