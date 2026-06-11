---
name: ssh-remote
description: Run commands and move files on remote hosts over SSH — execute a command and capture its output, upload/download files via SFTP, list a remote directory — with key or password auth, when a task needs to operate a server you reach over SSH rather than the local machine
triggers: [ssh, remote, server, scp, sftp, deploy, host, vps, paramiko, remote command]
tools: [code_exec, shell]
---

# ssh-remote — operate remote hosts over SSH

When a task needs to act on a *remote* machine — run a command on a VPS, deploy
to a server, pull a log file, restart a service — use this. It speaks SSH (run
commands) and SFTP (move files), with key or password auth. Runs through
`code_exec` (python).

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): installs `paramiko`. Use
`skill op=files ssh-remote` to find the bundle directory.

## The helper

`scripts/ssh.py` takes a JSON spec with `host`/`user` auth and an `op`, and prints
JSON. Ops:

```sh
# Run a command, capture output + exit code:
python scripts/ssh.py '{"op":"run","host":"1.2.3.4","user":"deploy",
  "key_path":"~/.ssh/id_ed25519","cmd":"docker ps --format \"{{.Names}}\""}'

# Upload a file (SFTP):
python scripts/ssh.py '{"op":"put","host":"1.2.3.4","user":"deploy",
  "key_path":"~/.ssh/id_ed25519","local":"app.tar.gz","remote":"/srv/app.tar.gz"}'

# Download a file:
python scripts/ssh.py '{"op":"get","host":"1.2.3.4","user":"deploy",
  "password":"$SSH_PW","remote":"/var/log/app.log","local":"app.log"}'

# List a remote directory:
python scripts/ssh.py '{"op":"ls","host":"1.2.3.4","user":"deploy","key_path":"~/.ssh/id_rsa","remote":"/srv"}'
```

### Spec fields
- `op` — `run` | `put` | `get` | `ls`.
- `host` (required), `port` (default 22), `user` (required).
- Auth: `key_path` (private key file) **or** `password`. A `key_pass` decrypts an
  encrypted key.
- `cmd` (run), `local` + `remote` (put/get), `remote` dir (ls), `timeout`
  (seconds, default 30).

### Output (JSON on stdout)
```
{ "ok": true, "op": "run", "exit_code": 0, "stdout": "...", "stderr": "" }
{ "ok": true, "op": "put", "local": "...", "remote": "..." }
{ "ok": true, "op": "ls", "entries": ["a","b/"] }
```
`ok` reflects the SSH transport; a command that exits non-zero still returns its
`exit_code`/`stderr` so you can read the failure.

## Security

- Prefer **key auth** over passwords. Pass a `password` via an env var you set
  first (`export SSH_PW=...` → `$SSH_PW`); the helper never echoes it back.
- The helper auto-accepts unknown host keys (AutoAddPolicy) for convenience — fine
  for hosts you own; for untrusted hosts, pin the host key in your own paramiko
  code instead.
- Running remote commands is outward-facing and can change a live server —
  double-check the host and command before a destructive `run`.

## Going further

The helper is a fast start, not a cage — for an interactive shell, port
forwarding, recursive SFTP, or jump hosts, use `paramiko` directly. Pairs with
**docker-services** (manage containers on a remote host: `run` a `docker …`
command) and **archive-tools** (zip locally, `put`, extract remotely). See
`reference/recipes.md`.
