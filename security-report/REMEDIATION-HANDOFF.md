# Security Remediation — Finalization Runbook

Date: 2026-06-24. Everything below is **implemented, tested, and verified green**
(full `go build ./...` + `go vet ./...` clean; ~150 packages in `kernel/ +
plugins/ + internal/` pass; committed LF form is gofmt-clean). This file is the
"what's left to land it" handoff — no further code work remains.

> ⚠️ **Do not commit while a second session is active in this working copy.**
> Branch/HEAD/index operations affect the whole shared repo and will tangle a
> concurrent session (see the project's own hard-won lesson). Land this only when
> this checkout is yours alone.

## 1. Code changes to commit (all security remediation)

New packages (reusable guardrails):
- `kernel/envscrub/` — scrubbed child-process env (V-001/V-002)
- `kernel/streamlimit/` — per-client concurrent-stream cap (V-009)
- `internal/ciguard/` — CI self-hosted fork-guard linter (V-004, V-007)

Modified / new files:
- `kernel/controlplane/config.go` — register `AGEZT_OVERSEER_FLEET_LOCK`
- `kernel/controlplane/configcenter_handler.go` (+`configcenter_secret_mask_test.go`) — mask `RatingSecret` over the API (V-013)
- `kernel/restapi/restapi.go` (+`restapi_test.go`), `mailbox.go`, `streamcap.go` (+`streamcap_test.go`) — admin-only daemon-global routes (V-011) + SSE cap (V-009)
- `kernel/webui/artifact_route.go` — SVG CSP sandbox (H-005); `streamcap.go` + `webui.go` — SSE cap (V-009)
- `plugins/providers/embed/embed.go`, `plugins/providers/voice/voice.go` — netguard client (H-006)
- `plugins/tools/overseertool/kernelsource.go` (+`fleet_lock_test.go`) — System-guardian guard + `AGEZT_OVERSEER_FLEET_LOCK` opt-in gate (V-012)
- `plugins/channels/discord/discord.go` (+`attachment_url_test.go`, `inbound_image*_test.go`) — attachment host allowlist (H-001)
- `docs/THREAT-MODEL.md` — "Hardening knobs (opt-in)" reference

(Also fixed in this tree by the concurrent session: V-006 Host/Origin, V-008
query-token, V-010 clamp, V-007 setup-go-safe, V-001/V-002 envscrub adoption.)

### Commit command (run only when the checkout is exclusively yours)

```bash
git add kernel/envscrub/ kernel/streamlimit/ internal/ciguard/ \
        kernel/controlplane/config.go kernel/controlplane/configcenter_handler.go kernel/controlplane/configcenter_secret_mask_test.go \
        kernel/restapi/ kernel/webui/streamcap.go kernel/webui/artifact_route.go kernel/webui/webui.go \
        plugins/providers/embed/embed.go plugins/providers/voice/voice.go \
        plugins/tools/overseertool/kernelsource.go plugins/tools/overseertool/fleet_lock_test.go \
        plugins/channels/discord/ docs/THREAT-MODEL.md security-report/
git rm list_vault.py
git commit -m "security: remediate audit findings (V-009/011/012/013, H-001/005/006, V-004/007 guards)"
```

Verify before commit (per the repo's pre-push rule — CRLF working tree masks
plain `gofmt -l`, so use the git-tracked form):

```bash
go vet ./...
git ls-files '*.go' | xargs gofmt -l   # expect empty
```

## 2. Owner actions (not code — your decision/credentials)

- [ ] **Rotate** the live `MINIMAX_API_KEY` / `DEEPSEEK_API_KEY` that were in `.env`; move `AGEZT_VAULT_PASSPHRASE` to an OS secret store. Tighten perms:
      `icacls .env /inheritance:r /grant:r "$env:USERNAME:(R)"`
- [ ] **Enable the fleet-admin gate** if wanted: `AGEZT_OVERSEER_FLEET_LOCK=on`
      (off by default; blocks agent-initiated `overseer` edit/create of agents).
- [ ] **CI infra:** move self-hosted runners to ephemeral + require approval for
      fork PRs (the `internal/ciguard` lint backstops the per-job guard).
- [ ] **Track** `emersion/go-imap/v2` for its stable release.
- [ ] **Tidy** root scratch scripts (`scout_*`, `find_*`, `update_scout*`) into
      an ignored local dir when convenient.

## 3. Already done (no action)
- `.playwright-mcp/` secret-bearing console captures — **purged**.
- `list_vault.py` — **removed from disk** (stage with `git rm`, in the commit above).
- Hardening knobs — **documented** in `docs/THREAT-MODEL.md`.
