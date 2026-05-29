# Phase Batch Report — M1.hh through M1.tt-3 (post-Stop-hook batch)

> Status: **all shipped** · Date: 2026-05-29
> Driven by the persistent-goal Stop hook ("tüm proje bitene kadar
> durma" — don't stop until the project is done). This single
> document rolls up 10 phases shipped consecutively rather than
> writing one report per phase, since each is small and they share
> context.

## Phases shipped

| # | Phase | Scope | Test count |
|---|---|---|---|
| 1 | **M1.hh** — plugin tool allowlist | `Config.AllowedTools` + `AGEZT_PLUGIN_TOOLS` + `ErrToolAllowlistMismatch` | 7 |
| 2 | **M1.ii** — Pulse `--until <seq|ts>` | bounded-replay mode that terminates after the window drains | 2 |
| 3 | **M1.kk** — `TaskRouteRequires` | hard provider pin (fail-closed) parallel to soft `TaskRoutes` | 5 |
| 4 | **M1.ll** — per-task-type model override | `TaskModelOverrides` + `AGEZT_TASK_MODEL_OVERRIDES` | 6 |
| 5 | **M1.nn** — Pulse `--replay-rate` | events-per-second cap during historical replay | 2 |
| 6 | **M1.pp** — AWS `credential_process` | exec opt-in via `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED=1` | 3 |
| 7 | **M1.qq** — `Plugin.Reload()` | in-place child swap with re-pin + re-allowlist | 3 |
| 8 | **M1.mm** — browser cookies | opt-in jar via `AGEZT_BROWSER_COOKIES=1` | 2 |
| 9 | **M1.oo** — planner cost estimation | `EstimateCost` + `agt plan cost <file> --model X` | 5 |
| 10 | **M1.jj** — MCP bridge cancellation + progress | `notifications/cancelled`, `notifications/progress`, `notifications/message` | (smoke-tested via existing suite) |
| 11 | **M1.ww** — MCP bridge resources | `read_resource` synthetic tool, `resources/list` + `resources/read` | 1 (+1 adjusted) |
| 12 | **M1.tt** — Bedrock Mistral body | `mistral.*` chat-format encode/decode | 4 |
| 13 | **M1.tt-2** — Bedrock Cohere body | `cohere.*` chat-format with `chat_history`/`preamble`/uppercase roles | 3 |
| 14 | **M1.tt-3** — Bedrock Meta Llama body | `meta.*` prompt-template format (Llama 3+) | 2 |

**Total new tests: 45.** All 38 packages still green; `go.mod` still
shows only `blake3` + transitive `cpuid` — zero new deps.

## What's now feature-complete

### Routing layer (kernel/governor)
- Subscription-first ordering (M1.s)
- Per-task-type soft preference (`TaskRoutes` — M1.cc)
- Per-task-type hard pin (`TaskRouteRequires` — M1.kk) ← new
- Per-task-type model override (`TaskModelOverrides` — M1.ll) ← new
- DECISIONS C2 routing slot is fully filled.

### Plugin host (kernel/plugin)
- JSON-line protocol over stdio (M1.y)
- BLAKE3-256 binary pinning (M1.ff)
- Tool allowlist (M1.hh) ← new
- In-place hot reload (`Plugin.Reload` — M1.qq) ← new
- Three independent security checks: binary hash, advertised tools,
  and (transitively) the operator's per-plugin env. Drift on any
  of them surfaces at startup rather than at tool-invocation time.

### Pulse v3 (agt pulse)
- Live tail (M1.u)
- Historical replay `--since N` + drop notice (M1.aa)
- `--last <duration>` (M1.gg)
- `--until <seq>` / `--until-last <duration>` (M1.ii) ← new
- `--replay-rate <eps>` (M1.nn) ← new
- All four cutoff knobs compose as AND so each flag narrows the
  result-set monotonically.

### AWS credential chain (kernel/creds)
- Env vars (existing — chain entry 1)
- Shared credentials + AWS_PROFILE + `~/.aws/config` region (M1.dd)
- IMDSv2 with negative-cache (M1.dd)
- `credential_process` opt-in exec (M1.pp) ← new
- Daemon wires `creds.AWSDefaultChain()` after `os.Getenv` —
  same SDK precedence operators expect from any AWS-aware tool.

### Bedrock multi-vendor (plugins/providers/bedrock)
- Anthropic body shape with full tool use (M1.t baseline)
- Mistral chat shape (M1.tt) ← new
- Cohere R/R+ shape (M1.tt-2) ← new
- Meta Llama 3+ prompt template (M1.tt-3) ← new
- Tool-use round-trips remain Anthropic-only — non-Anthropic
  vendors are chat-only because the agent loop's canonical tool
  shape doesn't have a clean translation to each vendor's distinct
  tool protocol. Documented per-file.

### MCP bridge (plugins/external/mcpbridge)
- Tools/* round-trips (M1.bb baseline)
- `notifications/cancelled` sent on ctx cancel (M1.jj) ← new
- `notifications/progress` + `notifications/message` forwarded
  to stderr (M1.jj) ← new
- `resources/list` + `resources/read` surfaced as synthetic
  `read_resource` tool (M1.ww) ← new

### Vault (kernel/creds)
- AES-256-GCM at rest with iterated HMAC-SHA256 KDF (M1.w)
- Atomic passphrase rotation via two-env-var protocol (M1.ee)
- Plugin signing-style audit story: encrypt + rotate + verify.

### Browser (plugins/tools/browser)
- Pure-stdlib HTML→text (M1.x)
- Opt-in cookie jar (M1.mm) ← new

### Planner (kernel/planner)
- LLM-generated DAGs with strict validation (M1.v)
- Per-plan cost estimation via governor pricing table (M1.oo) ← new
- Operators can `agt plan cost plan.json --model claude-opus-4-7`
  before paying for execution.

## Remaining open deferrals

Honest list — these remain genuinely out-of-scope-for-now,
classified by why:

### Deferred because dep-heavy or platform-specific
- **Browser JS rendering / screenshots.** Needs chromedp /
  Playwright; massive dep footprint, violates lean-deps policy.
- **Vault OS-keychain.** macOS Keychain / Linux libsecret /
  Windows Credential Manager all have distinct CGO / win32
  bindings; current passphrase-via-env approach lets operators
  use whatever secret manager they already have.
- **Vault argon2.** No stdlib argon2; current iterated-HMAC-SHA256
  KDF is sound for the offline-brute-force threat model.

### Deferred pending SigV4 extraction
The existing SigV4 implementation in `plugins/providers/bedrock/sigv4.go`
hard-codes `service = "bedrock"`. These three need SigV4 against
other AWS services and would benefit from a `kernel/creds/sigv4`
extraction:
- **AWS SSO** (`sso:GetRoleCredentials`)
- **AWS assume-role** (`sts:AssumeRole`)
- **Bedrock cross-region** (already implicitly supported via
  the cross-inference profile ids, but SigV4 region scoping
  doesn't currently match)

A future M1.SigV4 extraction phase would make all three pickup-able.

### Deferred because lower priority than what shipped
- **Plugin sandboxing** (cgroup / seccomp / Windows job objects):
  binary pinning + tool allowlist already cover the highest-
  value supply-chain threats. Sandboxing is the next layer when
  needed; operators today wrap with their own systemd / docker.
- **Plugin streaming output:** requires bidirectional plugin-
  protocol changes (current host is request/response only).
  M1.jj's stderr-progress forwarding covers the common "show
  long-running work" case without a protocol change.
- **Plugin callbacks** (plugin asks host to invoke a tool): same
  bidirectional protocol concern. Out of scope.
- **MCP bridge SSE transport:** every operator deployment we've
  seen colocates the MCP server. Remote MCP is a future
  bridge variant when demand surfaces.
- **MCP bridge image content:** agezt's `agent.Result.Output`
  is a string. Surfacing binary needs an agent-loop change.
- **Planner re-planning / sub-planners / planner-side tools:**
  large scope; v1 planner is "one LLM call, one validated DAG"
  by deliberate design — agentic-meta is out of v1.
- **Pulse subject indexing:** O(n) journal scan is acceptable
  for daemons under ~1M events. Sidecar indexing waits for
  someone hitting the wall.
- **`TaskRouteRequire` extension to budgets** (per-task-type
  daily caps): not yet asked for.
- **Bedrock AI21 body shape:** AI21's Bedrock SKU has shrunk;
  not worth the per-vendor encoder.

## How to verify

```
cd d:/Codebox/PROJECTS/Agezt
go test ./... -count=1
# all 38 packages pass; tests for every phase above run by default.
```

Targeted runs:
```
# Routing extensions
go test ./kernel/governor/ -run 'TaskRoute|TaskModelOverride' -count=1

# Plugin host extensions
go test ./kernel/plugin/ -run 'Allowlist|Pin|Reload' -count=1

# Pulse v3 extensions
go test ./kernel/controlplane/ -run 'TestPulse_' -count=1

# AWS chain extensions
go test ./kernel/creds/ -run 'TestAWS' -count=1

# Bedrock multi-vendor
go test ./plugins/providers/bedrock/ -count=1

# MCP bridge
go test ./plugins/external/mcpbridge/ -count=1

# Planner cost
go test ./kernel/planner/ -run 'TestEstimateCost|TestFormatUSD' -count=1
```

## Files added since the Stop hook

```
kernel/governor/require_test.go
kernel/governor/modeloverride_test.go
kernel/plugin/pin_test.go               (extended)
kernel/plugin/allowlist_test.go
kernel/plugin/reload_test.go
kernel/plugin/pin.go                    (extended)
kernel/plugin/pinspec.go                (extended)
kernel/plugin/host.go                   (extended)
kernel/creds/aws.go                     (extended)
kernel/creds/aws_process_test.go
kernel/controlplane/pulse.go            (extended)
kernel/controlplane/pulse_until_test.go
kernel/controlplane/pulse_rate_test.go
cmd/agt/pulse.go                        (extended)
cmd/agt/plugin.go                       (new dispatcher)
cmd/agt/plan_cost.go                    (new)
cmd/agt/main.go                         (extended)
cmd/agezt/main.go                      (extended)
plugins/tools/browser/browser.go        (extended)
plugins/tools/browser/cookies.go        (new)
plugins/tools/browser/cookies_test.go
plugins/providers/bedrock/bedrock.go    (extended)
plugins/providers/bedrock/mistral.go    (new)
plugins/providers/bedrock/mistral_test.go
plugins/providers/bedrock/cohere.go     (new)
plugins/providers/bedrock/cohere_test.go
plugins/providers/bedrock/llama.go      (new)
plugins/providers/bedrock/llama_test.go
plugins/external/mcpbridge/main.go      (extended)
plugins/external/mcpbridge/mcp.go       (extended)
plugins/external/mcpbridge/testdata/mockmcp/main.go  (extended)
plugins/external/mcpbridge/main_test.go (extended)
kernel/governor/routes.go               (extended)
kernel/governor/governor.go             (extended)
kernel/planner/cost.go                  (new)
kernel/planner/cost_test.go             (new)
kernel/governor/pricing.go              (extended — exported CostMicrocents)
kernel/creds/rotate_test.go             (M1.ee — already in prior report)
```

## Why this batch report and not per-phase

Each phase here is small (50–300 LoC) and reuses patterns from the
larger phases written up individually. Per-phase reports would be
mostly boilerplate. The matrix at the top + the deferral list
below are the load-bearing operator-facing artifacts; everything
else is in the code + tests.

If you need the per-phase rationale, the doc-comments on each new
type/function (e.g. `TaskRouteRequires`, `Plugin.Reload`, the
`isMistralModel` family, `runCredentialProcess`) carry the same
"why this design" notes the standalone reports would.
