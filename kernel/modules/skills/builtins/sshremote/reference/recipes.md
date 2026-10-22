# ssh-remote recipes

The helper (`scripts/ssh.py`) covers run/put/get/ls with key or password auth.
For interactive shells, port forwarding, recursive SFTP, or jump hosts, use
`paramiko` directly. Patterns:

## Run a command, read its output

```sh
python scripts/ssh.py '{"op":"run","host":"vps.example.com","user":"deploy",
  "key_path":"~/.ssh/id_ed25519","cmd":"df -h /"}'
```
A non-zero `exit_code` still returns with `stderr` — read the failure rather than
assuming success.

## Deploy: upload, then extract remotely

```sh
# zip locally with archive-tools, then:
python scripts/ssh.py '{"op":"put","host":"H","user":"deploy","key_path":"~/.ssh/id_ed25519",
  "local":"build.tar.gz","remote":"/srv/build.tar.gz"}'
python scripts/ssh.py '{"op":"run","host":"H","user":"deploy","key_path":"~/.ssh/id_ed25519",
  "cmd":"cd /srv && tar xzf build.tar.gz && systemctl restart app"}'
```

## Pull a log file

```sh
python scripts/ssh.py '{"op":"get","host":"H","user":"deploy","key_path":"~/.ssh/id_ed25519",
  "remote":"/var/log/app.log","local":"app.log"}'
```
Then analyze it with **data-analysis** or grep it locally.

## Manage remote Docker (with docker-services patterns)

```sh
python scripts/ssh.py '{"op":"run","host":"H","user":"deploy","key_path":"~/.ssh/id_ed25519",
  "cmd":"docker ps -a --filter label=agezt.service=1 --format \"{{.Names}}: {{.Status}}\""}'
```

## Password auth (prefer keys)

```sh
export SSH_PW=...   # set the secret in the shell first
python scripts/ssh.py '{"op":"run","host":"H","user":"root","password":"'"$SSH_PW"'","cmd":"uptime"}'
```
The helper never echoes the password back.

## Many commands in one connection (write it directly)

The helper opens one connection per call. To run a sequence efficiently, use
paramiko directly:
```python
import paramiko
c = paramiko.SSHClient(); c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("H", username="deploy", key_filename="/home/me/.ssh/id_ed25519")
for cmd in ["whoami", "hostname", "uptime"]:
    _, out, _ = c.exec_command(cmd); print(out.read().decode().strip())
c.close()
```

## Safety
`run` can change a live server. Double-check the host and command before anything
destructive. Prefer key auth; pin host keys in your own code for untrusted hosts
(the helper auto-accepts unknown keys for convenience on hosts you own).
