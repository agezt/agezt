# PHASE M893 — Built-in ssh-remote skill bundle

**Status:** shipped
**Milestone:** M893 (this session's range M889–M899; branched from `origin/main`,
concurrent session's local-main arc untouched).
**Theme:** Backlog **#34** — a thirteenth built-in skill bundle: operate remote
hosts over SSH (run commands, move files), extending the machine-control reach
beyond the local box.

## What shipped

A built-in `ssh-remote` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (`builtinBundles` + `go:embed` only):

- `SKILL.md` — the ops, key-vs-password auth, the host-key auto-accept caveat,
  and the "remote run can change a live server — double-check" safety note.
- `scripts/setup.sh` — `pip install paramiko`.
- `scripts/ssh.py` — one JSON-spec helper over paramiko, four ops: `run`
  (exec a command → exit_code/stdout/stderr), `put`/`get` (SFTP upload/download),
  `ls` (remote dir listing with trailing `/` for dirs). Key (`key_path`,
  optional `key_pass`) or `password` auth; the password is never echoed back;
  `ok` reflects the transport while a non-zero command still returns its code.
- `reference/recipes.md` — run+read output, deploy (put → extract remotely),
  pull a log, manage remote Docker (with the docker-services `agezt.service=1`
  label), password auth, multi-command via paramiko, safety.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/`. The seeder auto-loads it.
It tests in isolation: `go test ./plugins/builtinskills/`. Branched from
`origin/main` (my M862–M892), leaving the concurrent session's diverged local
`main` (their M880–M901 arc, preserved on the feat/m891 branch) untouched.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile ssh.py` passes; `sh -n setup.sh`
  clean. Package suite green — `TestSeedAll_InstallsSSHRemote` asserts the bundle
  seeds **active** and materializes `ssh.py` / `setup.sh` / `recipes.md`;
  bundle-count assertions now cover thirteen bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` deliberately skipped.

## Notes
- Thirteen seeded bundles now ship. ssh-remote extends docker-services to remote
  hosts (run a `docker …` over SSH) and pairs with archive-tools (zip → put →
  extract) for deploys. Remote `run` is outward-facing — the SKILL flags the
  host/command double-check.
