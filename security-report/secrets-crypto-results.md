# Secrets and Crypto Results - AGEZT

Date: 2026-06-24

## Secrets

Gitleaks found 5 redacted secret-shaped findings in ignored local files:

- `.env:4` - API-key shaped value
- `.env:7` - API-key shaped value
- `.playwright-mcp/page-2026-06-23T21-32-16-195Z.yml:627` - JWT-shaped value
- Two duplicate decoded AWS-token patterns on the same Playwright snapshot line

Manual infrastructure review also noted local plaintext secret material in
`.env`. These files are ignored and not tracked, but local plaintext secrets
should still be rotated/removed if live.

Machine-readable output: `gitleaks.json`.

## Crypto

No confirmed cryptographic implementation vulnerability was found in the manual
review. Positive controls include:

- Web/control-plane token comparisons use constant-time comparisons.
- Agent gateway JWT verification pins expected metadata and validates HMAC.
- Session IDs and control tokens are generated with cryptographic randomness.

Rejected scanner noise:

- `gosec` G101 hits were environment variable names, issuer/audience constants,
  or test/example placeholders, not embedded production credentials.
- `gosec` G404 hits were retry/governor jitter and not used for security
  decisions.

See `verified-findings.md` for remediation.
