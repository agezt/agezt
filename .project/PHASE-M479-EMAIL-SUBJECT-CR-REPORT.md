# M479 — Email: strip bare CR from the Subject header

## Context
The email channel derives the `Subject:` header from the first line of the outbound
text (`subjectFor`). Subject content is agent/observer-influenced (brief titles,
`agt send`, the `notify` tool).

## The bug (LOW)
The first line was cut at the first `'\n'` only:

```go
firstLine := out.Text
if i := strings.IndexByte(firstLine, '\n'); i >= 0 { firstLine = firstLine[:i] }
firstLine = strings.TrimSpace(firstLine)
```

A full `CRLF` is neutralized (the `\n` terminates the cut and `TrimSpace` strips a
trailing `\r`), but a **lone interior `\r`** survives into the `Subject:` line —
`subjectFor("Hello\rBcc: evil@example.com")` → `"Agezt: Hello\rBcc:
evil@example.com"`. RFC-compliant MTAs require `CRLF` as the header boundary so a
bare CR is usually not treated as a new header, making practical Bcc/extra-header
injection unlikely — but it is a non-conformant header and a latent risk against
lenient parsers. Severity LOW.

## The fix
Cut at the first CR **or** LF, so the subject is genuinely a single line with no
CR/LF:

```go
if i := strings.IndexAny(firstLine, "\r\n"); i >= 0 { firstLine = firstLine[:i] }
```

## Test + negative control
`plugins/channels/email/email_test.go`: `TestSubjectFor` gains
`"Hello\rBcc: evil@example.com" → "Agezt: Hello"` and `"first\r\nsecond" → "Agezt:
first"`.

**Negative control:** reverting to `IndexByte('\n')` made the bare-CR case fail —
`subjectFor("Hello\rBcc: evil@example.com") = "Agezt: Hello\rBcc: evil@example.com"`.
Restored; test passes.

## Verification / gate
- `plugins/channels/email` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
