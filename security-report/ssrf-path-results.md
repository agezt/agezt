# SSRF and Path Traversal Results - AGEZT

Date: 2026-06-24

## SSRF

No generic unauthenticated server-side fetch endpoint was confirmed.

Positive controls:

- Agent HTTP tool uses a host allowlist.
- Netguard blocks loopback, private, link-local/cloud metadata, unspecified,
  multicast/broadcast, CGNAT, and embedded IPv4/NAT64-style pivots.
- Redirect hops are rechecked for host policy and dial-level address policy.

Hardening:

- `H-001` in `verified-findings.md`: Discord attachment fetches are signed and
  channel-allowlisted, but should still validate `https` plus Discord/CDN host
  policy or use netguard.

Rejected scanner noise:

- CLI registry/Home Assistant URL fetches are operator-configured local client
  behavior, not remote attacker-controlled SSRF.

## Path Traversal

No exploitable server-side path traversal was confirmed.

Positive controls:

- Control-plane sandbox project/file routes use `confineUnder`.
- Sandbox deletion requires a direct child project path.
- Artifact download content is addressed by ref and MIME/filename output is
  constrained.

Rejected scanner noise:

- CLI file reads flagged by gosec are local user-supplied CLI arguments, not
  remote server-side path traversal.
