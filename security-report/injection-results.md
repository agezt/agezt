# Injection Scanner Results (non-command injection)

> Scanner domain: SQLi, NoSQLi, SSTI, XXE, LDAP injection, HTTP header injection / response splitting / CRLF.
> Repo: `D:\Codebox\PROJECTS\AGEZT`. Scope excludes `node_modules`, `frontend/dist`, `*_test.go`, and the
> duplicate `.worktrees\rebased-main\` / `.claude\worktrees\` trees (stale copies of the canonical source).

## Summary

**No exploitable injection vulnerabilities found in the six domains in scope.** The codebase is structurally
inhospitable to most of these classes (no DB/ORM, no template engine, no LDAP client, no entity-resolving XML
parser), and the two genuine candidate sinks (outbound email headers, downloadable filename header) are already
sanitized against CRLF. Findings below are all **Info** (surface-confirmation / defense-in-depth notes), not
defects.

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High     | 0 |
| Medium   | 0 |
| Low      | 0 |
| Info     | 5 |

---

## Surface-by-surface justification

### SQL Injection (CWE-89) — NOT APPLICABLE
No SQL database, no ORM, no `database/sql`. Grep for `database/sql|sqlx|gorm|sqlite|mongo|redis\.|bbolt`
returned only false positives: a toolbox catalog string, `kernel/redact` connection-string *redaction*
patterns, and a `kernel/configcenter/classifier.go` keyword list. Persistence is append-only JSONL journal +
JSON state buckets (`kernel/journal`, `kernel/state`). **No SQL query is constructed anywhere.** Zero surface.

### NoSQL Injection (CWE-943) — NOT APPLICABLE
No MongoDB/Redis/Elasticsearch/CouchDB/DynamoDB client. The `find(`/`$where`/`aggregate` operators the
sc-nosqli checklist targets do not appear against any datastore. Zero surface.

### Server-Side Template Injection (CWE-1336) — NOT APPLICABLE
**No Go template engine is used at all.** A precise grep for `"text/template"`, `"html/template"`,
`template.New`, `template.Must`, `.Execute`, `.ExecuteTemplate` returned **zero matches** across the tree.
The broad `.Parse(` hits were all `url.Parse` (webui.go:1129, discord.go:405, update.go:539/578) — URL
parsing, not template compilation. The web console is a `go:embed`-ded static React SPA (`kernel/webui/dist`),
served as bytes; no server-side rendering of user input into template code. Zero surface.

### XML External Entity (CWE-611) — NOT EXPLOITABLE
The only XML parsing is Go's stdlib `encoding/xml` via `xml.Unmarshal`:
- `kernel/creds/sts.go:208`, `kernel/creds/web_identity.go:160` — parse AWS STS XML *responses* (trusted AWS
  endpoint, netguard-guarded).
- `plugins/channels/wecom/wecom.go:187,486` — parse the WeCom inbound webhook envelope (untrusted), under a
  64 KiB `LimitReader` body cap, and the payload is AES-decrypted + HMAC-signature-gated before use.

Go's `encoding/xml` **does not resolve external entities or DTDs** — `<!ENTITY xxe SYSTEM "file://...">`,
out-of-band `%xxe;`, and the billion-laughs DoS all do not apply (the parser does not expand custom entity
references and ignores DOCTYPE). This is the documented sc-xxe false-positive #3 ("Go encoding/xml — does not
process external entities by default"). No `xml.NewDecoder` with a custom entity map exists. Not exploitable.

### LDAP Injection (CWE-90) — NOT APPLICABLE
**No LDAP/directory client exists.** `go-ldap`, `ldap.Dial`, `ldap.Search`, `InitialDirContext`,
`NewSearchRequest` all return zero matches. The earlier broad `ldap`-substring grep matched only on words like
"Directory" in unrelated code. Authentication is operator-token / password / OAuth — no directory bind. Zero
surface.

### HTTP Header Injection / Response Splitting / CRLF (CWE-93 / CWE-113) — DEFENDED
See Info findings below. Two real sinks interpolate variables into header-shaped output; both strip CR/LF.
All other `w.Header().Set(...)` / `req.Header.Set(...)` calls set static constants, framework-derived content
types, operator-configured bearer tokens, or HMAC signatures — none reflect untrusted message content into a
header *value*. Go's `net/http` additionally rejects `\r`/`\n` in header field values on write, and
`net/smtp.SendMail` rejects newlines in envelope addresses — a second backstop under every sink here.

---

## Info Findings (defense-in-depth confirmations — not defects)

### INJ-INFO-01 — Outbound email Subject header: CRLF cut (defended)
- **File:** `plugins/channels/email/email.go:195` (sink), `:218-221` (`subjectFor`)
- **CWE:** CWE-93 (Email/Header Injection) — *mitigated*
- **Confidence:** 95
- The Subject is built via `fmt.Fprintf(&b, "Subject: %s\r\n", subject)`. The subject derives from the first
  line of agent/reply body text (potentially attacker-influenced via an inbound channel message). `subjectFor`
  explicitly cuts at the first `\r` **or** `\n` (`strings.IndexAny(firstLine, "\r\n")`), with an in-code comment
  referencing prior fix M479 that closed exactly the "lone interior `\r` survives" variant. Header injection
  into Subject is blocked. **No action needed.**

### INJ-INFO-02 — Outbound email To/From headers (allowlist-gated, not attacker-controlled)
- **File:** `plugins/channels/email/email.go:193-194`
- **CWE:** CWE-93 — *not reachable*
- **Confidence:** 90
- `To:` interpolates `to`, which must be an **exact-match key** in the operator-configured, fail-closed
  recipient allowlist (`channel.Allowlist.Allows`, `kernel/channel/channel.go:134`); an attacker driving an
  outbound run can only target pre-listed addresses. `From:` is operator config (`c.from`). The allowlist map
  trims whitespace but doesn't strip interior CRLF, so the *only* path to a CRLF-bearing `To` is the operator
  placing a malformed address into their own allowlist — operator-trusted config, not an external vector. The
  default transport `net/smtp.SendMail` also rejects newlines in the envelope `to`/`from` it is handed.
  **Residual hardening (optional, low value):** reject addresses containing `\r`/`\n` at allowlist-build time
  in `NewAllowlist`.

### INJ-INFO-03 — Artifact download Content-Disposition: filename sanitized (defended)
- **File:** `kernel/webui/artifact_route.go:52-54`, `sanitizeFilename` `:79-90`
- **CWE:** CWE-113 — *mitigated*
- **Confidence:** 95
- `?download=1&name=<...>` flows into `Content-Disposition: attachment; filename="..."`. `sanitizeFilename`
  strips `\\`, `/`, `"`, `\n`, `\r` before interpolation, so neither CRLF response-splitting nor quote-breakout
  is possible. The stored MIME is independently allowlisted (`safeContentType`), SVG gets a sandbox CSP, and
  `nosniff` is global. Solid. **No action needed.**

### INJ-INFO-04 — Outbound webhook custom headers (internal values + framework CRLF reject)
- **File:** `kernel/webhook/webhook.go:213-215`; `plugins/channels/webhook/webhook.go:297-299`
- **CWE:** CWE-113 — *not reachable*
- **Confidence:** 85
- `X-Agezt-Event`/`X-Agezt-Subject` are set from `ev.Kind` (internal event-kind constants) and `ev.Subject`
  (bus-pattern-validated via `bus.ValidatePattern`). Even if a crafted value reached `req.Header.Set`, Go's
  `http.Header.Write`/transport rejects header values containing `\r` or `\n`. No untrusted free-form text is
  placed into a header value. Not exploitable.

### INJ-INFO-05 — SMS `r.Host` used in signature input, not a header sink (no host-header injection)
- **File:** `plugins/channels/sms/sms.go:263-274` (`signedURL`), `:256`
- **CWE:** CWE-113 (Host header injection) — *not applicable*
- **Confidence:** 85
- `r.Host` + `X-Forwarded-Proto` reconstruct the public URL **only as input to Twilio HMAC-SHA1 signature
  verification** (`twilioSignature`), never reflected into a response header, redirect `Location`, or a
  password-reset-style URL/email. Forging `Host` only makes the locally computed signature diverge from the
  attacker's forged one (which they cannot validly produce without Twilio's auth token). The code already
  prefers an operator-set `PublicURL` when configured. Not an injection vector. (The web console separately
  enforces a `Host` allowlist + same-origin mutation guard in `kernel/webui/webui.go:1025`.)

---

## Method notes
- Discovery greps run for: template sinks (`template.New/Parse/Execute`, `text/template`, `html/template`),
  XML decoders (`xml.NewDecoder`, `xml.Unmarshal`), LDAP clients, DB/ORM engines, response-header writes
  (`w.Header().Set/Add`, `http.Redirect`, `http.SetCookie`, `Set-Cookie`, `Content-Disposition`), raw mail/
  protocol header construction (`fmt.Fprintf` into `To:`/`Subject:`/`From:`), and `Host`/`X-Forwarded-Host`
  reflection.
- Each candidate sink was opened and traced source→sink; auto-protections (Go `net/http` header validation,
  `net/smtp` address validation, stdlib `encoding/xml` no-entity-resolution) were credited per the skill
  false-positive guidance.
- Stale duplicate trees under `.worktrees\` / `.claude\worktrees\` were excluded to avoid double-reporting.
