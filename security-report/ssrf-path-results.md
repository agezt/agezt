# SSRF / Path-Traversal / File-Upload — Server-Side Findings

Scanner: server-side hunter (SSRF · path traversal/LFI/RFI · zip-slip · insecure file upload)
Repo: AGEZT (Go kernel/daemon + plugins + React console), `D:\Codebox\PROJECTS\AGEZT`, branch `main`.
Method: discovery grep (`http.Client`/`Do`/`NewRequest`, `net.Dial`, `filepath.Join`/`os.Open`/`ReadFile`/`WriteFile`, `archive/zip`+`tar`, `url.Parse`) → verify guard coverage + input trust on each outbound/file sink.

> Note: a `.worktrees/rebased-main` checkout in-tree contains an older copy of this report. Its sole substantive finding (L1, self-update download not netguarded) is **already fixed on current `main`** — see "Notable change vs. prior pass" below. The duplicate worktree files were excluded from this analysis.

## Executive summary

**This domain is strongly hardened. No Critical / High / Medium SSRF, path-traversal, zip-slip, or file-upload vulnerabilities were found.** Two Informational notes only.

Severity counts: **Critical 0 · High 0 · Medium 0 · Low 0 · Informational 2.**

The SSRF defense (`kernel/netguard`, dialer-level `Control` hook) is correct and comprehensive: it validates the *resolved* IP on the initial dial **and every redirect hop** (defeating DNS-rebinding TOCTOU and redirect-to-internal), collapses IPv6-embedded v4 (NAT64 `64:ff9b::/96`, IPv4-compat) so the metadata IP can't be smuggled as an IPv6 literal, and blocks loopback, RFC1918+ULA, link-local/`169.254` (cloud metadata), CGNAT `100.64/10`, the `0.0.0.0/8` zero-block, multicast and broadcast; it fails closed on an unresolved/unparseable dial address. Every agent- or channel-reachable outbound sink builds its client from netguard:

- Agent tools: `http`, `fetch`, `web_search`, `browser.read` — all netguard, http/https-scheme-only, body-capped; `http`/`browser` additionally re-check their host allowlist on every redirect hop (`CheckRedirect`) so a 302 can't carry `Authorization` to an un-allowlisted host.
- `kernel/mcp/http.go` (remote MCP), `kernel/market/sync.go`, `kernel/catalog/sync.go`, `plugins/providers/embed`, channel-OAuth token exchange (`kernel/controlplane/channel_oauth.go`, strictest variant — blocks even loopback), outbound webhook dispatcher (`cmd/agezt/main.go:5041` builds the netguard client and passes it via `webhook.WithClient`), and the OneBot inbound-media fetch (`plugins/channels/onebot/onebot.go:97`, the one channel that fetches an attacker-supplied URL — uses strict `netguard.New()`).

Operator-pinned destinations correctly do **not** need the guard because the agent cannot choose the host: `homeassistant` tool, `peer`/`remote_run` mesh (host from `AGEZT_PEERS`), STT/TTS (`AGEZT_STT_URL`/`AGEZT_TTS_URL`), provider API bases, and all ~34 channel API bases. The Matrix `mxc://` media fetch (`plugins/channels/matrix/matrix.go:352`) is safe: the attacker-controlled `server`/`mediaId` are `url.PathEscape`d into the *path* of the operator's fixed homeserver base — they cannot change the host.

Path confinement is consistently correct and bears the scars of prior hardening (M171/M252/M253/M427/M440): the `file` tool resolves its root with `Abs`+`EvalSymlinks`, re-resolves every requested path, resolves the deepest existing ancestor for not-yet-existing paths (closing the symlinked-parent gap), uses `O_NOFOLLOW` on write and an `Lstat` symlink guard + atomic temp-rename on `replace`, and re-checks each symlink during `search`/`glob` walks (`entryEscapesRoot`). The skill `BundleStore` (`kernel/skill/bundle.go`, `cleanRel`) rejects absolute and `..` paths. The artifact store (`kernel/artifact/artifact.go`) is content-addressed with a strict 64-hex `validRef`, so no caller-supplied string ever forms a filesystem path.

The only archive extraction in the tree is the CLI backup restore (`cmd/agt/backup.go:465`), which is zip-slip-safe: `isAllowedBackupPath` rejects `..`/absolute/backslash and requires a known subtree, then a `filepath.Join(cleanDest,…)` + `HasPrefix(cleanDest+sep)` containment check, with `O_EXCL` create and refusal to write into a non-empty home. Bundle export/import ships files as a JSON map (not an archive), each entry validated through `cleanRel` before write.

File upload: `/api/transcribe` (`kernel/webui/transcribe.go`) caps at 25 MiB via `MaxBytesReader`, reads the `file` form field into memory and forwards it to the STT backend — it is never written to a path derived from the upload filename, so there is no upload-path traversal or webroot-write surface. The artifact-raw server (`kernel/webui/artifact_route.go`) sanitizes the served `Content-Type` to a safe allowlist (everything else → `application/octet-stream`, plus global `nosniff`), sandboxes SVG via per-response CSP, and sanitizes the download filename — addressing stored-content rendering risks.

---

## INFO-1 — STT/TTS/voice client is not netguard-wrapped (operator-config destination)

- **Severity:** Informational · **CWE:** CWE-918 (partial)
- **File:** `kernel/stt/stt.go:54-56` (nil `HTTPClient` → plain `&http.Client{}`), wired from `cmd/agezt/main.go` `sttTranscriberFromEnv` / voice adapter.
- **Detail:** The STT client and the OpenAI-compatible voice adapter build a stock `http.Client` with no egress guard. The endpoint comes from `AGEZT_STT_URL` / `AGEZT_TTS_URL` (and provider base), which is **operator configuration**, not agent- or channel-controlled input — the agent supplies only the audio bytes / text, never the host. So this is not an agent-reachable SSRF. Recorded only because the embeddings adapter (`plugins/providers/embed`) and every other provider-style outbound path that takes an operator URL *do* wrap netguard (loopback/private allowed, link-local blocked) for parity and defense-in-depth.
- **Remediation (defense-in-depth, optional):** Build the STT/TTS client from `netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(timeout)` so a typo'd or compromised STT URL can't reach `169.254.169.254`. Low priority.
- **Confidence:** High that the guard is absent; High that it is not agent-exploitable.

## INFO-2 — `file` tool `withinRoot` uses a `..` string-prefix containment check (correct here)

- **Severity:** Informational · **CWE:** CWE-22 (not exploitable)
- **File:** `plugins/tools/file/file.go:807-818` (`withinRoot`).
- **Detail:** Containment is asserted via `filepath.Rel(root, child)` then rejecting `rel == ".."` or `strings.HasPrefix(rel, "..")`. Both arguments are already `Abs`+`EvalSymlinks`-canonicalized, and `Rel` never emits a `..`-prefixed result for a true in-root descendant, so the check is sound (the prefix test can only over-reject a sibling like `..foo`, which is the safe direction). No fix needed; flagged only because string-prefix path checks are a frequent bug site and this one is correct.
- **Confidence:** High (not exploitable).

---

## Notable change vs. prior pass

`kernel/update/update.go` (self-update binary download) — the prior worktree report flagged this as a Low SSRF (plain `http.Client`, no netguard). On current `main` it is **fixed**: `New()` (lines 134-173) now builds a netguard-guarded client (`AllowLoopback`+`AllowPrivate`, link-local/metadata still blocked) with a `CheckRedirect` that enforces HTTPS on every hop, layered over the existing SHA256 + Ed25519 integrity verification. No longer a finding.

---

## Verified-safe surfaces (reviewed, no issue)

| Surface | File | Why safe |
|---|---|---|
| `http` / `browser.read` tools | `plugins/tools/http/http.go`, `browser/browser.go` | http/https-only, host allowlist (default-deny), netguard every hop, allowlist re-checked on redirect |
| `fetch` / `web_search` tools | `plugins/tools/fetch/fetch.go`, `websearch/websearch.go` | netguard client; fetch http/https-prefix + 50 MiB cap; web_search fixed DuckDuckGo endpoint |
| Remote MCP | `kernel/mcp/http.go` | netguard (loopback/private allowed for local servers, link-local blocked); URL operator-config + scheme-validated (`kernel/mcp/store.go`) |
| Marketplace / catalog sync | `kernel/market/sync.go`, `kernel/catalog/sync.go` | netguard; `resolveRef` refuses off-host + non-http; SHA256 + Ed25519 verify; JSON only (no disk-path writes) |
| Embeddings adapter | `plugins/providers/embed/embed.go` | netguard (loopback/private allowed for local Ollama); URL operator-config |
| Channel OAuth token exchange | `kernel/controlplane/channel_oauth.go` | strict `netguard.New()` (blocks loopback too); instance URL https-validated then dial-guarded; `redirect_uri` only forwarded to provider, never a daemon `Location` |
| Outbound webhook dispatcher | `kernel/webhook/webhook.go` + `cmd/agezt/main.go:5041` | operator `AGEZT_WEBHOOKS` sinks; daemon injects netguard client via `WithClient` |
| OneBot inbound media (attacker URL) | `plugins/channels/onebot/onebot.go:97` | dedicated strict `netguard.New()` mediaClient; test asserts `file://`/SSRF refused |
| Matrix / Telegram / WhatsApp / Line / WeCom / Feishu / iMessage media | respective channel files | endpoint built against fixed operator API base; attacker id/server is path/query-escaped, cannot change host |
| `homeassistant` tool, `peer`/`remote_run` mesh, STT/TTS, push/notify, provider bases | respective files | destination operator-pinned config; agent cannot choose host |
| `file` tool (read/write/list/search/glob/replace/delete) | `plugins/tools/file/file.go` | Abs+EvalSymlinks root; per-path `resolve`+`withinRoot`; deepest-ancestor resolution for new paths; `O_NOFOLLOW` write + Lstat symlink guard on replace; per-walk symlink re-check; atomic temp+rename |
| Skill bundle store (export/import + `skill cat`) | `kernel/skill/bundle.go`, `kernel/controlplane/skill.go:306` | `cleanRel` rejects abs/`..`; size-capped; staged temp-dir swap; `handleSkillReadFile` passes path straight to validated `Read` |
| Artifact store + `/api/artifact/raw` | `kernel/artifact/artifact.go`, `kernel/webui/artifact_route.go` | content-addressed 64-hex `validRef` (no input→path); served Content-Type allowlisted, SVG CSP-sandboxed, download filename sanitized, nosniff |
| `/api/transcribe` upload | `kernel/webui/transcribe.go` | 25 MiB `MaxBytesReader`; bytes forwarded to STT, never written to an upload-derived path |
| Backup create / restore (only archive extraction) | `cmd/agt/backup.go:465` | zip-slip-safe: `isAllowedBackupPath` + Join prefix check + `O_EXCL`; CLI/operator-run; refuses non-empty home |
| Static file serving | (none) | no `ServeFile`/`FileServer`/`http.Dir`; console is `go:embed` |

**Intentional-by-design, not flagged (per brief):** `kernel/mcp/http.go` / `embed` / `market` allowing loopback+private for legitimate local servers; the per-tool `AllowLoopback`/`AllowPrivate` opt-in flags (operator-set, default off on agent tools); the overall default-allow capability posture.
