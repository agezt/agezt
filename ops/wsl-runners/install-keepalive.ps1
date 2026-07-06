<#
.SYNOPSIS
    Installs (or reinstalls) the Windows-side WSLKeepAlive scheduled task on
    host WHITE. Persistent WSL session holder that prevents the
    systemd-logind poweroff cycle that kills long-running CI jobs.

.DESCRIPTION
    1. Writes (or overwrites) the launcher .cmd to %USERPROFILE%
       using the robust version from ops\wsl-runners\wsl-keepalive.cmd
       (nohup + disown so the inner sleep survives the wsl.exe process
       being signalled when WSL terminates the instance).
    2. Removes any existing WSLKeepAlive task (idempotent).
    3. Registers the WSLKeepAlive scheduled task: AtLogOn trigger,
       AllowStartIfOnBatteries + DontStopIfGoingOnBatteries + zero
       execution-time-limit, RestartCount 999 with RestartInterval 1m
       (the holder will respawn itself if anything ever does kill it).
    4. Starts the task immediately so the fix takes effect right now.
    5. Verifies: task is Running, ≥1 wsl.exe process is alive.

.PARAMETER CmdPath
    Override the launcher .cmd path (default: %USERPROFILE%\wsl-keepalive.cmd).

.PARAMETER RepoRoot
    Path to the repo root (default: auto-resolved via $PSScriptRoot or PWD).
    The script copies ops\wsl-runners\wsl-keepalive.cmd from here.

.EXAMPLE
    # Standard install (run as the user who owns the WSL Ubuntu install):
    powershell -NoProfile -ExecutionPolicy Bypass -File ops\wsl-runners\install-keepalive.ps1

.EXAMPLE
    # Verify it stays running:
    Get-ScheduledTask -TaskName 'WSLKeepAlive'
    Get-Process -Name wsl
    # (inside WSL)
    loginctl list-sessions
    ps -ef | grep '[s]leep 3600'
#>
[CmdletBinding()]
param(
    [string]$CmdPath = (Join-Path $env:USERPROFILE 'wsl-keepalive.cmd'),
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..' '..')).Path
)

$ErrorActionPreference = 'Stop'

# Resolve the source cmd from the repo (always copy the canonical version
# from ops/wsl-runners/, so updates to the repo are reflected on reinstall).
$Src = Join-Path $RepoRoot 'ops' 'wsl-runners' 'wsl-keepalive.cmd'
if (-not (Test-Path $Src)) {
    throw "Source launcher not found: $Src (run this from the repo root or pass -RepoRoot)"
}

Write-Host "===1. Installing launcher to $CmdPath==="
Copy-Item -Path $Src -Destination $CmdPath -Force
Write-Host "Done."

Write-Host "===2. Removing any pre-existing WSLKeepAlive task (idempotent)==="
$existing = Get-ScheduledTask -TaskName 'WSLKeepAlive' -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "Existing task (State=$($existing.State)) — removing for clean re-register."
    Unregister-ScheduledTask -TaskName 'WSLKeepAlive' -Confirm:$false
}

Write-Host "===3. Registering WSLKeepAlive task (AtLogOn, durable, auto-restart)==="
$trigger  = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$action   = New-ScheduledTaskAction -Execute $CmdPath
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -RestartCount 999 `
    -RestartInterval (New-TimeSpan -Minutes 1)

Register-ScheduledTask `
    -TaskName 'WSLKeepAlive' `
    -Trigger $trigger `
    -Action $action `
    -Settings $settings `
    -Description 'Holds a persistent WSL2 session so systemd-logind never poweroffs the VM (reproducible fix from ops/wsl-runners/).' `
    -Force | Out-Null

Write-Host "Done."

Write-Host "===4. Starting the task now==="
Start-ScheduledTask -TaskName 'WSLKeepAlive'
Start-Sleep -Seconds 3

Write-Host "===5. Verifying==="
$t = Get-ScheduledTask -TaskName 'WSLKeepAlive'
$i = $t | Get-ScheduledTaskInfo
Write-Host ("TaskName : " + $t.TaskName)
Write-Host ("State    : " + $t.State)
Write-Host ("Action   : " + $t.Actions[0].Execute)
Write-Host ("Trigger  : " + $t.Triggers[0].CimClass.CimClassName)
Write-Host ("LastRC   : " + $i.LastTaskResult)

$wslProcs = @(Get-Process -Name 'wsl' -ErrorAction SilentlyContinue)
Write-Host ("wsl.exe process count: " + $wslProcs.Count)
if ($wslProcs.Count -lt 1) {
    Write-Warning "No wsl.exe processes running — the holder is not holding. Check the task LastRC above."
}

Write-Host ""
Write-Host "===Next: verify from inside WSL that the VM stops poweroff-ing==="
Write-Host "  wsl -d Ubuntu -- bash -lc 'journalctl --since -2min | grep -c InitTerminateInstanceInternal'"
Write-Host "  (expected: 0 — the in-VM keepalive journal should show no restarts after the task is running)"
Write-Host ""
Write-Host "Install complete."
