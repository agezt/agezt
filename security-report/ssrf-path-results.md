# SSRF / Path Traversal / Archive / Upload / Open-Redirect Hunt — Results

Codebase: AGEZT (Go + React), `D:\Codebox\PROJECTS\AGEZT`
Scope: outbound-request paths that bypass `kernel/netguard`; file read/write/serve from request/agent/channel input; archive extraction (zip-slip); upload handlers; open-redirect.

## Executive summary

This area is **strongly hardened**. The netguard dialer-level guard (DNS-rebind + per-redirect-hop safe, NAT64/IPv4-compat collapse, CGNAT/zero-block/broadcast coverage) is wired into every *attacker/agent-reachable* outbound sink: the `http`, `fetch`, `web_search`, `browser` tools, marketplace sync, catalog/Ollama discovery, channel-OAuth token exchange, the webhook dispatcher, and the OneBot inbound-media fetch. Operator-pinned destinations (home-assistant tool, peer mesh, whatsappgw/WAHA, voice STT/TTS base URLs, channel API bases, push targets) correctly do **not** need the guard because the agent cannot choose the host.

Path-confinement is consistently and correctly implemented and has visibly survived prior hardening passes (M171/M252/M253/M427/M440, UPD-001/002): the `file` tool (symlink-resolved root, TOCTOU `O_NOFOLLOW`, per-walk symlink re-check, atomic replace), codeexec (`slug()` + `sanitizeRelFile` rejecting `..`/abs/colon/NUL), the sandbox controlplane reader (`confineUnder`), and the backup restore (zip-slip-safe `isAllowedBackupPath` + `filepath.Join` prefix check + `O_EXCL`). The only archive extraction in the tree (backup restore, CLI/operator-run) is safe. No `http.FileServer`/`ServeFile`/`http.Dir` exists; the webui is `go:embed`. The OAuth callback renders a self-closing HTML page with no user-controlled `Location` (no open redirect); no `next=`/`return_to` redirect exists.

**No High or Critical exploitable issues found.** Two Low / Informational notes below.

---

## L1 — Self-update binary download does not route through netguard (operator-config-gated, integrity-verified)

- **Severity:** Low (Informational)
- **CWE:** CWE-918 (SSRF), partial
- **Location:** `kernel/update/update.go:131-160` (plain `http.Client`, no netguard), `kernel/update/update.go:542-585` (`downloadBinary`), `:401-490` (`checkGitHub`/`checkEndpoint`)
- **Attack:** The update download URL (`UpdateInfo.URL`) comes from a remote manifest fetched from the configured update *Endpoint* (or the GitHub release `browser_download_url`). The download client is a stock `http.Client` with only a `requireHTTPS` redirect hook — it is **not** netguard-guarded. A party who controls the configured update endpoint could return a manifest whose `url` points at an internal host, turning the daemon into a blind GET probe of internal HTTPS services.
- **Why it is Low, not High:**
  - The update *Endpoint*/*GitHubOwner/Repo* are **operator configuration**, not agent- or channel-reachable input. An attacker must already control the operator's chosen update source.
  - `requireHTTPS` is enforced on the initial URL **and every redirect hop** (`CheckRedirect`), blocking `http://`, `file://`, `gopher://`, and HTTPS→HTTP downgrade.
  - The fetched binary is verified by **SHA256** and (when a public key is configured) **Ed25519 signature** before any swap; a mismatched/internal response cannot become the running binary.
  - Net effect is limited to a TLS-only blind GET to an attacker-named host, only meaningful as an internal port-scan and only by someone who already owns the update channel.
- **Impact:** Blind internal HTTPS reachability probe; no metadata-endpoint pivot (TLS-only, and metadata is plaintext HTTP/169.254); no RCE (integrity-gated).
- **Confidence:** High that the guard is absent here; Low that it is practically exploitable.
- **Remediation (defense-in-depth):** Build the update client from `netguard.New().HTTPClient(timeout)` (keeping the `requireHTTPS` CheckRedirect) so a malicious/compromised update endpoint cannot aim the download at loopback/RFC1918/link-local. Optionally restrict the download host to the manifest's own host or an allowlist.

---

## L2 — `file` tool `withinRoot` uses a `..` string-prefix check (correct here; noted for robustness)

- **Severity:** Informational
- **CWE:** CWE-22 (Path Traversal) — not exploitable
- **Location:** `plugins/tools/file/file.go:807-818` (`withinRoot`)
- **Detail:** `withinRoot` computes `filepath.Rel(root, child)` and rejects when `rel == ".."` or `strings.HasPrefix(rel, "..")`. The `HasPrefix(rel, "..")` test would also reject a sibling like `..foo` whose `Rel` begins with `..` — but that is the *safe* (over-reject) direction and `Rel` never emits a `..`-prefixed result for an in-root descendant, so containment is sound. Both inputs are already `filepath.Abs`+`EvalSymlinks`-canonicalized before this call, and the new-file path resolves the deepest existing ancestor (M253), closing the symlinked-parent gap. No fix required; recorded only because string-prefix containment checks are a common bug site and this one is correct.
- **Confidence:** High (not exploitable).

---

## Verified-safe surfaces (reviewed, no issue)

| Surface | File(s) | Why safe |
|---|---|---|
| `http` agent tool | `plugins/tools/http/http.go` | scheme allowlist (http/https), host allowlist (default-deny unless AllowAll), netguard client on every hop |
| `fetch` agent tool | `plugins/tools/fetch/fetch.go` | http/https-only prefix check, netguard client, 50 MiB cap |
| `web_search` tool | `plugins/tools/websearch/websearch.go` | fixed DuckDuckGo lite endpoint, netguard client |
| `browser` tool | `plugins/tools/browser/browser.go` | netguard (per redirect_test) |
| Marketplace sync | `kernel/market/sync.go` | netguard (loopback/private allowed for self-host, link-local blocked), `resolveRef` refuses off-host & non-http; packs are JSON (no archive/disk-path writes); SHA256 + Ed25519 verify |
| Catalog / Ollama discovery | `kernel/catalog/sync.go`, `discovery.go` | guardedClient; endpoint from env |
| Channel OAuth token exchange | `kernel/controlplane/channel_oauth.go` | `netguard.New().HTTPClient`; `instance_url` https-validated then dial-guarded; `redirect_uri` only forwarded to provider, never used as daemon `Location` |
| OAuth callback page | `kernel/webui/webui.go:619-657` | renders self-closing HTML; error msg `htmlEscape`d; **no** user-controlled redirect → no open redirect |
| Outbound webhook dispatcher | `kernel/webhook/webhook.go` | operator-config sinks (`AGEZT_WEBHOOKS`), daemon passes netguard client (`WithClient`) |
| OneBot inbound media | `plugins/channels/onebot/onebot.go:375` | attacker-controlled CQ URL fetched via dedicated `netguard.New().HTTPClient`; test asserts `file://` and SSRF refused |
| Matrix mxc media | `plugins/channels/matrix/matrix.go:352` | endpoint built against fixed operator `Homeserver` base; attacker `server`/`mediaId` are `url.PathEscape`d into the path, cannot change host |
| Telegram media | `plugins/channels/telegram/telegram.go:317-356` | fixed `api.telegram.org` base; `file_id` query-escaped |
| home-assistant tool, peer mesh, whatsappgw, voice STT/TTS, push, line/zalo/imessage/signal/mastodon/nextcloudtalk | respective channel/tool files | destination is operator-pinned config; agent cannot choose host (documented, acceptable) |
| `file` tool (read/write/list/search/glob/replace/delete) | `plugins/tools/file/file.go` | `filepath.Abs`+`EvalSymlinks` root; per-path `resolve()` + `withinRoot`; new-path deepest-ancestor resolution; `O_NOFOLLOW` write & `Lstat` symlink guard on replace; per-walk `entryEscapesRoot` symlink re-check; atomic temp+rename |
| codeexec sandbox writes | `plugins/tools/codeexec/codeexec.go`, `runtimes.go:168,193` | project name via `slug()` ([a-z0-9-] only); extra files via `sanitizeRelFile` (rejects abs/`..`/colon/NUL) |
| Sandbox controlplane reader | `kernel/controlplane/sandbox.go` | `confineUnder` (rejects abs/`..`/NUL, Windows-separator prefix check); delete requires exact `<root>/<segment>` |
| Artifact get/list/delete | `kernel/controlplane/artifact.go` | content-addressed 64-hex `ref` (validated `ErrBadRef`), no filesystem path from input |
| Storage stats | `kernel/controlplane/storage.go` | read-only walk of fixed home subdirs, no input path |
| Backup create / restore (only archive extraction in tree) | `cmd/agt/backup.go:465-530` | zip-slip-safe: `isAllowedBackupPath` (no `..`/abs/backslash, must be known subtree) + `filepath.Join(cleanDest,…)` prefix check + `O_EXCL`; CLI/operator-run, refuses non-empty home |
| REST path-suffix params | `kernel/restapi/mailbox.go:236`, `restapi.go:518` | used as store-lookup IDs (message id / correlation id), never joined to a filesystem path |
| Static file serving | (none) | no `ServeFile`/`FileServer`/`http.Dir`; webui is `go:embed` |

**Intentional-by-design, not flagged (per brief):** `kernel/mcp/http.go` MCP remote allowing loopback/private; the default-allow capability posture; per-tool `AllowLoopback`/`AllowPrivate` flags (opt-in, operator-set).
