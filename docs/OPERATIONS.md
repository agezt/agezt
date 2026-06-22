# AGEZT Operations Guide

This document covers day-2 operations for a running AGEZT daemon: health
checks, metrics, triage, cost management, backup/restore, and the runbooks an
operator needs when something goes wrong. It is written for operators running
AGEZT as a resident process (server, VPS, or local workstation).

> This is a living document. It reflects the operational surfaces present in the
> source at the time of writing. Commands and flags may evolve; always check
> `agt <cmd> --help` for the current surface.

## Operational surface map

| Concern | Command / endpoint | Notes |
|---|---|---|
| Preflight health | `agt doctor [--strict] [--json]` | base dir, daemon reachability, version skew, journal integrity, tools |
| Status overview | `agt status [--json]` | daemon health, provider, model, channels, HTTP bindings |
| Liveness probe | `GET /healthz` | unauthenticated; process is alive |
| Readiness probe | `GET /readyz` | unauthenticated; can serve work (not halted) |
| Metrics | `GET /metrics` (token-authed) | Prometheus text format; `agezt_` prefix |
| Spend tracking | `agt budget [--json]` | global + per-task-type caps, strict-pricing posture |
| Budget control | `agt budget set <amount\|0\|off>` | adjust daily ceiling at runtime |
| Budget check | `agt budget check [--task-type <t>]` | headroom before a run; exit 3 if exhausted |
| Cache savings | `agt cache [--since <dur>] [--json]` | prompt-cache tokens + $ saved |
| Event audit | `agt why <event_id> [--payload]` | walk the BLAKE3-hash-chain correlation |
| Journal verify | `agt journal verify` | verify the hash chain integrity |
| Journal tail | `agt journal tail [N] [--json]` | recent events |
| Policy decisions | `agt edict log [N] [--denied]` | recent allow/deny audit |
| Policy stats | `agt edict stats [--since <dur>]` | denial rate + per-capability breakdown |
| Agent authority | `agt agent authority <slug> [--json]` | effective runtime policy proof |
| Backup | `agt backup [--out <file>]` | secret-free snapshot (journal + catalog) |
| Backup inspect | `agt backup inspect <file>` | read bundle manifest without restoring |
| Restore | `agt restore <file> [--home <dir>]` | restore a backup into a fresh home |
| Vault | `agt vault encrypt` / `agt vault rotate` | at-rest encryption + key rotation |
| Provider check | `agt provider check --all` | verify credentials + latency + cost |
| Halt / resume | `agt halt` / `agt resume` | freeze / clear all in-flight runs |
| Shutdown | `agt shutdown` | graceful exit (CI-friendly) |
| Live activity | `agt pulse` | real-time event tail |
| Run inspection | `agt runs list` / `agt runs show <id>` | run history + event arc |

---

## 1. Health and readiness

### `agt doctor` — the first thing to run

`agt doctor` is the preflight checklist. Run it when something feels wrong or
as a first step after boot.

```bash
agt doctor
agt doctor --strict    # exit non-zero on warnings too (CI/monitoring)
agt doctor --json      # machine-readable for automation
```

It checks:

- **base dir** — resolvable, writable
- **memory store** — not corrupt
- **daemon reachability** — control-plane socket recorded and responding
- **version skew** — CLI and daemon protocol versions match
- **journal integrity** — BLAKE3 hash chain verifies
- **tools** — registered tools are loadable
- **halt state** — whether the daemon is halted

Exit codes:

- `0` — nothing failed (warnings are advisories by default)
- `1` — at least one check failed (or a warning under `--strict`)
- `2` — usage error

### Liveness vs readiness (deployment probes)

AGEZT exposes two unauthenticated probe endpoints for container/k8s/systemd
orchestration:

```text
GET /healthz   → 200 {"status":"ok"}    (process is alive)
GET /readyz    → 200 {"status":"ready"} | 503 {"status":"not_ready","reason":"halted"}
```

- **Liveness** (`/healthz`): the HTTP server can answer. Use for liveness probes
  and uptime monitors. No state, no sensitive fields.
- **Readiness** (`/readyz`): the daemon can serve work right now. Returns 503 when
  halted. Use for readiness probes and load-balancer health checks so a halted
  daemon stays live but is pulled from rotation.

These are intentionally unauthenticated so deployment tooling can probe without
a credential. They expose only liveness/readiness — never version, model, or
run data.

---

## 2. Metrics

The `/metrics` endpoint exposes Prometheus-format metrics. Unlike `/healthz`
and `/readyz`, it is **token-authenticated** because it exposes spend and
activity volume.

```bash
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8800/metrics
```

Metrics use the `agezt_` prefix. The daemon injects the gauge source; this
package only formats. The set may grow; at the time of writing it includes
active run counts and related activity gauges. Check the live endpoint for the
current set.

### Prometheus scrape example

```yaml
scrape_configs:
  - job_name: "agezt"
    static_configs:
      - targets: ["localhost:8800"]
    authorization:
      credentials: "<your-daemon-token>"
```

### Grafana dashboard suggestion

A minimal dashboard should include:

- `agezt_active_runs` — gauge: runs in flight
- `agt budget` output — spend vs ceiling (scrape via textfile collector or a
  custom exporter that calls `agt budget --json`)
- `agt edict stats` output — denial rate over time
- `agt cache` output — cache savings trend

AGEZT does not ship a prebuilt Grafana dashboard JSON. Build one from the
metrics endpoint + the CLI JSON outputs above.

---

## 3. Cost management

### Daily spend tracking

```bash
agt budget              # current-day spend vs ceiling + per-task-type caps
agt budget --json       # machine-readable
```

The budget view shows:

- **global** spend vs daily ceiling (percentage)
- **pricing posture** — strict (unpriced models refused) or lax (charged $0)
- **per task type** — spend vs cap for each configured task type

### Adjusting the ceiling at runtime

```bash
agt budget set 25       # set daily ceiling to $25
agt budget set 0        # remove ceiling (unlimited)
agt budget set off      # same as 0
```

This requires the admin token; tenant tokens cannot change the budget.

### Pre-run headroom check

```bash
agt budget check                    # will the global budget allow a run?
agt budget check --task-type coding # will the coding task-type cap allow it?
```

Exit code `3` means the budget is exhausted — useful in CI/scripts to fail
before submitting a run that would exceed the cap.

### Strict pricing

```bash
AGEZT_PRICING_STRICT=on ./bin/agezt
```

When enabled, models with no known price are **refused** rather than silently
charged $0. This prevents quiet budget bypass via unpriced models. The posture
appears in `agt budget` output.

---

## 4. Policy and governance triage

### Denial audit

When an agent's tool call is refused, the decision is journaled as a
`policy.decision` event. Surface it with:

```bash
agt edict log               # recent decisions (allow + deny)
agt edict log --denied      # denials only
agt edict log 20 --json     # last 20, machine-readable
```

Each decision line shows: timestamp, verdict (allow / DENY / DENY-hard),
capability, tool, and reason.

### Denial aggregate

```bash
agt edict stats             # total, allowed, denied, denial rate
agt edict stats --since 1h  # last hour only
```

A **spike in denials** is a signal worth investigating — it may indicate prompt
injection, a misconfigured agent, or a policy regression.

### Effective authority per agent

```bash
agt agent authority <slug>          # merged profile + policy overlay
agt agent authority <slug> --json   # machine-readable
```

This shows the effective runtime authority: tool allow/deny, trust ceiling,
memory scope, workdir, config overrides, approval mode, capability levels (with
ceiling-cap annotations), and the hard-deny floor. Use it to verify that
displayed authority matches enforced authority.

---

## 5. Event audit and forensics

### Walking the causality chain

Every run, tool call, wake, and policy decision is an event in a
tamper-evident, BLAKE3-hash-chained journal.

```bash
agt why <event_id>              # events sharing the correlation
agt why <event_id> --payload    # include payload bodies
agt why <event_id> --json       # full JSON for jq piping
```

The output includes the causation provenance chain (the events that *caused*
this one, root first) and a parent-correlation backlink for sub-agent runs.

### Journal integrity

```bash
agt journal verify              # verify the hash chain
```

Run this periodically (or in monitoring) to detect journal corruption early.
`agt doctor` also runs this as one of its checks.

### Recent events

```bash
agt journal tail 50             # last 50 events
agt journal tail 50 --json      # machine-readable
```

---

## 6. Backup and restore

### Backing up

`agt backup` writes a **secret-free** snapshot of the home directory (journal +
catalog). It runs **offline** (daemon stopped) and never includes credentials
or tokens.

```bash
# Stop the daemon first for a consistent snapshot.
agt shutdown

# Create the backup.
agt backup --out agezt-backup-$(date +%Y%m%d).tar.gz

# Restart the daemon.
./bin/agezt
```

The backup captures:

- `journal/` — the full event log (source of truth)
- `catalog/` — the model catalog (network-synced, not journaled)

It deliberately excludes:

- `creds.json` — provider credentials
- `runtime/control.token` — the daemon admin token
- Projections (rebuilt from the journal on next boot)

### Inspecting a backup

```bash
agt backup inspect agezt-backup-20260620.tar.gz
agt backup inspect agezt-backup-20260620.tar.gz --json
```

This reads the manifest (journal head seq + hash, included subtrees) and lists
the contents without unpacking — so you can verify a bundle before restoring it
onto a fresh host.

### Restoring

```bash
agt restore agezt-backup-20260620.tar.gz --home /path/to/fresh-home
```

Restore unpacks the journal and catalog into a fresh home. Projections rebuild
from the journal on the next daemon boot. Secrets must be re-provisioned
manually (they were never in the backup).

### Backup/restore drill

Run this drill periodically to verify your backup is restorable:

```bash
# 1. Back up the running home.
agt shutdown
agt backup --out /tmp/drill-backup.tar.gz

# 2. Restore into a fresh temp home.
agt restore /tmp/drill-backup.tar.gz --home /tmp/drill-home

# 3. Boot the daemon against the restored home and verify.
AGEZT_HOME=/tmp/drill-home ./bin/agezt &
sleep 2
AGEZT_HOME=/tmp/drill-home agt doctor
AGEZT_HOME=/tmp/drill-home agt journal verify

# 4. Clean up.
AGEZT_HOME=/tmp/drill-home agt shutdown
rm -rf /tmp/drill-home /tmp/drill-backup.tar.gz
```

If `doctor` and `journal verify` pass against the restored home, the backup is
sound.

---

## 7. Vault and credential management

### Encrypt the vault at rest

```bash
agt vault encrypt
```

Credentials are stored in an AES-256-GCM envelope. Once encrypted, the daemon
needs the passphrase to decrypt at boot. Never store the passphrase in the same
location as the vault.

### Rotate the passphrase

```bash
agt vault rotate
```

Rotation re-derives the key with the new passphrase and re-encrypts the vault.
Run this periodically and whenever a passphrase may have been exposed.

### Verify credentials

```bash
agt provider check          # check the configured provider
agt provider check --all    # check all registered providers
```

This performs a live roundtrip (latency + cost) so you can confirm credentials
work before relying on them.

---

## 8. Halt, resume, and shutdown

### Halt — freeze everything instantly

```bash
agt halt --reason "investigating an incident"
```

Halt freezes all in-flight runs. New runs are refused. The daemon stays alive
and `/healthz` keeps answering, but `/readyz` returns 503 so a load balancer
pulls it from rotation.

### Resume — clear the halt

```bash
agt resume --reason "incident resolved"
```

### Shutdown — graceful exit

```bash
agt shutdown
```

Signals the daemon to drain and exit cleanly. Use this in CI and for planned
restarts. The daemon waits for in-flight handlers to finish.

---

## 9. Live monitoring

### Pulse — real-time event tail

```bash
agt pulse                        # live tail of all events
agt pulse --correlation <id>     # tail one run's event chain
agt pulse --since 0 --replay-rate 50   # historical replay at 50x
```

### Runs — history and inspection

```bash
agt runs list                    # recent runs
agt runs show <correlation_id>   # a run's full event arc + stats
agt runs stats                   # aggregate run statistics
```

---

## 10. Incident triage runbooks

### "An agent did something unexpected"

```bash
# 1. Find the run.
agt runs list

# 2. Walk its event chain.
agt why <event_id> --payload

# 3. Check policy decisions for that run.
agt edict log --json

# 4. Check the agent's effective authority.
agt agent authority <slug>

# 5. If needed, halt the daemon to stop further action.
agt halt --reason "investigating"
```

### "Spend is higher than expected"

```bash
# 1. Check current spend.
agt budget

# 2. Check which task types are consuming.
agt budget --json | jq '.per_task[]'

# 3. Tighten the ceiling immediately if needed.
agt budget set 5

# 4. Enable strict pricing to block unpriced models.
# (restart with AGEZT_PRICING_STRICT=on)
```

### "A tool call was denied unexpectedly"

```bash
# 1. Find the denial.
agt edict log --denied

# 2. Check the capability's trust level.
agt edict show

# 3. Check the agent's effective authority.
agt agent authority <slug>

# 4. Walk the event chain for context.
agt why <event_id>
```

### "Journal corruption suspected"

```bash
# 1. Verify the chain.
agt journal verify

# 2. If corrupt, run doctor for a full diagnostic.
agt doctor

# 3. Restore from the latest backup if needed.
agt shutdown
agt restore agezt-backup-<latest>.tar.gz --home <fresh-dir>
```

### "Plugin crashed or is misbehaving"

```bash
# 1. List loaded plugins.
agt plugin list

# 2. Check the daemon log for plugin errors.
# (the daemon log is at $AGEZT_HOME/daemon.log)

# 3. Disable or remove the plugin.
agt plugin remove <name>

# 4. The daemon supports hot-reload; verify the plugin reloaded cleanly.
agt plugin list
```

---

## 11. Multi-tenant operations

When `AGEZT_MULTITENANT=on`, the daemon serves isolated tenant kernels.

```bash
agt tenant create <id>       # create an isolated tenant
agt tenant list              # list tenants
agt tenant token <id>        # reveal the per-tenant token
agt tenant rm <id>           # remove a tenant
```

Tenant-aware commands accept `--tenant <id>`:

```bash
agt budget --tenant <id>
agt edict log --tenant <id>
agt why <event_id> --tenant <id>
```

The admin token authorizes any tenant; a tenant token authorizes only its own.
See `docs/THREAT-MODEL.md` T8 for tenant boundary caveats.

---

## 12. Monitoring checklist

Minimum recurring checks for a production deployment:

| Frequency | Check | Command |
|---|---|---|
| Every scrape | Liveness + readiness | `/healthz`, `/readyz` |
| Every scrape | Metrics | `/metrics` |
| Hourly | Spend vs budget | `agt budget` |
| Hourly | Denial rate | `agt edict stats --since 1h` |
| Daily | Journal integrity | `agt journal verify` |
| Daily | Doctor (strict) | `agt doctor --strict` |
| Weekly | Backup drill | see §6 |
| Monthly | Vault rotation | `agt vault rotate` |
| Monthly | Credential verification | `agt provider check --all` |

---

## 13. Example operator wiring

AGEZT exposes probes, metrics, and CLI checks; it does not install your monitoring stack. Use these as starting points, not bundled defaults.

### Prometheus scrape sketch

```yaml
scrape_configs:
  - job_name: agezt
    metrics_path: /metrics
    static_configs:
      - targets: ["127.0.0.1:8787"]
```

Pair `/metrics` with `/healthz` and `/readyz` probes. Keep auth/tunnel policy consistent with your deployment; do not expose daemon control surfaces publicly without the controls in `docs/THREAT-MODEL.md` and `docs/OPERATIONS.md`.

### Grafana starter panels

Build panels from `/metrics` and CLI JSON outputs for:

- daemon readiness and health;
- active/running runs;
- daily spend vs budget;
- policy denials per hour;
- approval queue size;
- provider fallback/retry counts;
- journal verification failures;
- doctor/repair events;
- mailbox/help escalation counts.

### Backup scheduling examples

Cron-style example:

```cron
# Daily AGEZT backup at 03:15 local time.
15 3 * * * /usr/local/bin/agt backup --out /var/backups/agezt/agezt-$(date +\%Y\%m\%d).tar.zst >/var/log/agezt-backup.log 2>&1
```

Systemd timer deployments should run the same `agt backup` command from a locked-down service account and periodically run a restore drill against a scratch `AGEZT_HOME`.

### Vault rotation checklist

- Schedule a monthly reminder for `agt vault rotate`.
- Verify providers after rotation with `agt provider check --all`.
- Keep recovery material outside the AGEZT config directory.
- Record the rotation in your operator change log.

### Platform-specific validation notes

- Linux CI should cover warden/resource-limit behavior.
- Windows CI should cover path, shell, and service behavior where process isolation downgrades to timeout/output/env/workdir controls.
- macOS CI should cover developer install and filesystem behavior if macOS is a supported operator target.

---

## What is explicitly out of scope

- AGEZT does not ship a prebuilt Grafana dashboard. Build one from `/metrics`
  + CLI JSON outputs.
- AGEZT does not provide built-in alerting. Wire the metrics/probes into your
  existing monitoring stack (Prometheus, Grafana, Datadog, etc.).
- AGEZT does not auto-rotate vault passphrases. The operator must run
  `agt vault rotate` on a schedule.
- AGEZT does not auto-backup. The operator must schedule `agt backup` (cron,
  systemd timer, etc.).
