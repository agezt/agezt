# Access Control Results - AGEZT

Date: 2026-06-24

## Summary

No Critical or High access-control bypass was confirmed under the default
loopback posture. A Medium CSRF/DNS-rebinding issue was verified for the web
console's default password-session mode.

Verified controls:

- Web console data routes use token/session auth wrappers.
- Control-plane requests use a bearer token compared with constant-time compare.
- Agent gateway JWT verification pins algorithm/type/issuer/audience and checks
  HMAC with `hmac.Equal`.
- Sandbox file routes confine project/file paths under the sandbox root.
- REST/OpenAI-compatible APIs gate non-health routes with bearer auth.

Verified findings and hardening:

- `V-006` in `verified-findings.md`: password-session web console auth lacks a
  Host allowlist and mutating-route Origin/Sec-Fetch-Site checks, leaving a DNS
  rebinding CSRF path.
- `V-008` in `verified-findings.md`: query-string token fallback is accepted by
  all web data routes, while the SPA only needs it for SSE. Narrow the fallback
  to `/events` and token bootstrap routes.
- Exposing the web UI or REST/OpenAI API beyond loopback shifts risk to operator
  configuration. Use strict password mode and external TLS/auth controls for
  non-loopback deployments.

See `architecture.md` for the full route/auth map.
