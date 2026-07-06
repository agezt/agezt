# WSL Runner Operations

Infrastructure for the 3 self-hosted GitHub Actions runners (`wsl-runner-1/2/3`)
running in a WSL2 Ubuntu VM on host `WHITE`.

## Current setup

| Component | Location | Status |
|---|---|---|
| GitHub Actions runners | `/home/ersinkoc/actions-runner-{1,2,3}` | Registered as `wsl-runner-{1,2,3}-new` (IDs 25/26/27) |
| systemd unit files | `/etc/systemd/system/actions.runner.agezt-agezt.wsl-runner-{1,2,3}.service` | `Restart=always`, `RestartSec=10` |
| WSL VM config | `C:\Users\ersin\.wslconfig` | `vmIdleTimeout=999999999`, `autoMemoryReclaim=disabled` |
| Keepalive (in-VM) | `/etc/systemd/system/wsl-keepalive.service` | Continuous `sleep` process, `Restart=always` |
| Keepalive (Windows) | Scheduled task `WSLKeepAlive` → `wsl-keepalive.cmd` | Persistent WSL session (nohup+disown inside, foreground wsl.exe outside), at logon |
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

Run in PowerShell (one-time, per host). The script is idempotent and safe
to re-run; it copies the canonical launcher from `ops\wsl-runners\wsl-keepalive.cmd`
into `%USERPROFILE%`, re-registers the scheduled task with durable settings
(RestartCount 999 / RestartInterval 1 min), starts it immediately, and
verifies:

```powershell
# From the repo root:
powershell -NoProfile -ExecutionPolicy Bypass -File ops\wsl-runners\install-keepalive.ps1
```

### Why the naive `while true; do sleep 3600; done` version dies

A simpler one-liner
(`wsl.exe -d Ubuntu bash -c "while true; do sleep 3600; done"`) was
insufficient on the WSL2 build used here: when WSL terminates the instance
(after the last `wsl.exe` session ends, which `systemd-logind` treats as
"idle"), the `wsl.exe` process is signalled (exit code `0xC000013A` /
`STATUS_CONTROL_C_EXIT`), the bash loop dies, and the holder is gone. The
VM then boots fresh with no holder; the last transient `wsl.exe` session
ends; `systemd-logind` poweroffs again; the cycle repeats every ~1 min.

The robust launcher in `wsl-keepalive.cmd` uses `nohup ... & disown` to
**detach a persistent sleep inside the Ubuntu instance** (so it survives
the `wsl.exe` process being signalled) and then keeps a **foreground
`wsl.exe` sleep** alive too (so logind always sees ≥1 active session).
Together they hold the VM continuously across WSL poweroff/reboot cycles.

### Verify

Check that the VM stops cycling (the in-VM keepalive journal should show
no restarts after the task is running):

```bash
# Inside WSL:
journalctl -u wsl-keepalive.service --since -5min --no-pager
# Should show: "-- No entries --" (VM hasn't been killed)
```

```bash
# Also confirm the holder session is alive (≥1 wsl.exe process and a logind
# session for the Ubuntu user):
wsl -d Ubuntu -- bash -c "loginctl list-sessions && ps -ef | grep '[s]leep 3600'"
```

### Troubleshooting

If the VM is poweroff-ing (CI jobs killed mid-run with
"The runner has received a shutdown signal"), check these in order:

1. **Holder not running** (most common cause): the Windows-side task died
   or was never installed. Re-run `install-keepalive.ps1`. Verify
   `Get-ScheduledTask -TaskName 'WSLKeepAlive'` shows `State=Running` and
   `Get-Process -Name wsl` shows ≥1 process.
2. **systemd-logind poweroff signal** (the actual WSL2 mechanism): check
   the in-VM journal for `systemd-logind: The system will power off now!`
   followed by `Reached target poweroff.target`. If you see this, logind is
   seeing "no active sessions" and powering off. The holder above (nohup
   + foreground wsl.exe) is the fix; if you modified `wsl-keepalive.cmd`,
   re-run `install-keepalive.ps1` to restore the canonical version.
3. **dmesg poweroff attempts** (the WSL fallback when the in-VM shutdown
   stalls): `dmesg -T | grep InitTerminateInstanceInternal`. If events appear
   more often than every ~5 min, the holder is failing to hold — see #1.
4. **`vmIdleTimeout`** in `.wslconfig`: this is set to a large positive int
   (`999999999`) because this WSL build does not honor `-1`. If you see
   `vmIdleTimeout=-1` in the file, change it and run `wsl --shutdown` to
   force the re-read.

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