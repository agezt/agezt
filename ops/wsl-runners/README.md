# WSL Runner Operations

Infrastructure for the 3 self-hosted GitHub Actions runners (`wsl-runner-1/2/3`)
running in a WSL2 Ubuntu VM on host `WHITE`.

## Current setup

| Component | Location | Status |
|---|---|---|
| GitHub Actions runners | `/home/ersinkoc/actions-runner-{1,2,3}` | Registered as `wsl-runner-{1,2,3}-new` (IDs 25/26/27) |
| systemd unit files | `/etc/systemd/system/actions.runner.agezt-agezt.wsl-runner-{1,2,3}.service` | `Restart=always`, `RestartSec=10` |
| WSL VM config | `C:\Users\ersin\.wslconfig` | `vmIdleTimeout=-1`, `autoMemoryReclaim=disabled` |
| Keepalive (in-VM) | `/etc/systemd/system/wsl-keepalive.service` | Continuous `sleep` process, `Restart=always` |
| Keepalive (Windows) | Scheduled task `WSLKeepAlive` → `wsl-keepalive.cmd` | Persistent `wsl.exe` session at logon (required) |
| Go toolchain staging | `setup-go-safe` composite action | GOROOT/GOCACHE/GOTMPDIR on `/dev/shm` tmpfs |

## Keepalive service

The WSL2 VM suspends when no user-space process holds it open, killing
long-running CI jobs (e.g. the race detector's stressed depth pass). The
keepalive service runs a perpetual `sleep` process that makes the VM
ineligible for idle suspension.

### Install

From inside WSL Ubuntu (one-time):

```bash
bash ops/wsl-runners/install-keepalive.sh
```

### Verify

```bash
systemctl is-active wsl-keepalive.service
# → active
```

### How it works

- `ExecStart=/bin/sleep 3600` — a long sleep that costs 0 CPU and ~0 memory.
- `Restart=always` with `RestartSec=5` — systemd restarts the sleep when it
  exits (after 1h), creating a perpetual process inside the VM.
- Combined with `.wslconfig`'s `vmIdleTimeout=-1`, the VM stays alive between
  CI jobs, during idle periods, and through runner service restarts.

**Important:** the in-VM systemd keepalive alone is **not sufficient**. Windows
terminates the WSL2 VM process from outside regardless of in-VM activity. A
Windows-side scheduled task (below) is also required.

## Windows-side keepalive (required)

The WSL2 VM is a Windows process (`wslservice`). Windows kills it every
~15-50s when no foreground `wsl.exe` invocation holds a session open — even
with `vmIdleTimeout=-1` in `.wslconfig` and the in-VM systemd keepalive
running. The fix is a Windows scheduled task that holds a persistent WSL
session from the Windows side.

### Install

Run in PowerShell (one-time, per host):

```powershell
# 1. Create the launcher script
@'
@echo off
wsl.exe -d Ubuntu bash -c "while true; do sleep 3600; done"
'@ | Set-Content "$env:USERPROFILE\wsl-keepalive.cmd"

# 2. Register the scheduled task (starts at logon, runs indefinitely)
Register-ScheduledTask -TaskName 'WSLKeepAlive' `
  -Trigger (New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME) `
  -Action (New-ScheduledTaskAction -Execute "$env:USERPROFILE\wsl-keepalive.cmd") `
  -Settings (New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero)) `
  -Force

# 3. Start it now
Start-ScheduledTask -TaskName 'WSLKeepAlive'
```

### Verify

Check that the VM stops cycling (the in-VM keepalive journal should show
no restarts after the task is running):

```bash
# Inside WSL:
journalctl -u wsl-keepalive.service --since -5min --no-pager
# Should show: "-- No entries --" (VM hasn't been killed)
```

## Runner systemd units

Each runner's unit file (`actions.runner.agezt-agezt.wsl-runner-N.service`)
has:

```ini
[Service]
Restart=always
RestartSec=10
```

If the runner listener exits (clean exit, crash, or GitHub disconnect),
systemd restarts it within 10 seconds instead of leaving it offline.

### Modifying runner units

The unit files live at:
```
/etc/systemd/system/actions.runner.agezt-agezt.wsl-runner-{1,2,3}.service
```

After editing:
```bash
sudo systemctl daemon-reload
sudo systemctl restart actions.runner.agezt-agezt.wsl-runner-{1,2,3}.service
```

## Re-registering runners

If GitHub shows stale sessions ("A session for this runner already exists"):

1. Stop all runner services:
   ```bash
   sudo systemctl stop actions.runner.agezt-agezt.wsl-runner-*.service
   ```
2. Get a fresh registration token:
   ```bash
   gh api --method POST repos/agezt/agezt/actions/runners/registration-token --jq .token
   ```
3. Remove old config and re-register each runner:
   ```bash
   cd /home/ersinkoc/actions-runner-N
   rm -f .runner .credentials .credentials_rsaparams .runner_migrated .service .path
   ./config.sh --url https://github.com/agezt/agezt --token <TOKEN> \
     --name wsl-runner-N-new --labels wsl-runner --unattended --replace
   ```
4. Delete old runner registrations on GitHub and restart services.