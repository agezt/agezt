# M453 — Fuzz the catalog feed parser and the OpenAI-compat content parser

## Context
Extending fuzz coverage to two more untrusted-input parsers identified after the
M444–M449 arc:
- **Catalog `ParseAPIFile`** — the daemon ingests a models.dev-shaped `api.json`
  from the external feed (`agt catalog sync`) and from disk; an external/untrusted
  source.
- **OpenAI-compat message content** — `chatMessage.text()/images()/inputImages()`
  flatten a client's `content` field, which is either a string or an array of
  typed parts, via several `json.Unmarshal` shape attempts — custom parsing of
  fully untrusted network input on the daemon's API.

## What was added
- `kernel/catalog/fuzz_test.go` — `FuzzParseAPIFile`: arbitrary bytes → `ParseAPIFile`
  never panics (may error or return a Catalog), and a successful parse yields a
  non-nil Catalog with a non-nil Providers map.
- `kernel/openaiapi/fuzz_test.go` — `FuzzChatMessageContent`: arbitrary `content`
  bytes → `text()` / `images()` / `inputImages()` never panic.

Seeds cover the well-formed shapes (string content, text parts, image_url object
and string forms, provider map) plus `null`, `{}`, empty, wrong-typed arrays, and
raw binary.

## Verification
- **Seed runs**: both pass.
- **Fuzz runs** (`-fuzztime=20s` each):
  - catalog `ParseAPIFile` — **7,218,114** executions, PASS
  - openai `chatMessage` content — **10,306,528** executions, PASS
  No panic across ~17.5 M executions.
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. (No CHANGELOG entry — these add test coverage, no behaviour change; the
  fuzz arc as a whole is already recorded under M444–M449.)

## Fuzz coverage now (14 targets)
redaction, trust-ladder, journal reopen, control-plane pre-auth parse,
3× channel signature verify, 5× provider stream parse, **catalog feed parse**,
**openai-compat message content** — every untrusted/corrupt/external-feed input
parser in the daemon is now fuzz-hardened, all clean across ~100 M+ total
executions.
