# dev.ps1 - one-shot dev loop: isolated home + seeded vault + build + run.
#
#   .\dev.ps1                      build .\agezt.exe / .\agt.exe, seed .dev-home, run the daemon
#   .\dev.ps1 -Fresh               wipe .dev-home first (clean vault/journal/state)
#   .\dev.ps1 -SkipBuild           reuse .\agezt.exe / .\agt.exe from the last build
#   .\dev.ps1 -WebAddr 127.0.0.1:9000   serve the console elsewhere
#
# Frontend commands must be run from the repo's frontend directory:
#   cd .\frontend
#   npm run build     # runs: tsc --noEmit && vite build
#   npm test
# Running npm commands from the repo root will use the wrong working directory.
#
# The daemon runs against .\.dev-home (NEVER the real ~/.agezt - a dev run
# against the real home once rewrote live standing orders). Provider keys are
# seeded into the dev vault from .env (script dir, or the main repo root when
# running from a worktree): every KEY ending in _API_KEY, plus AGEZT_PROVIDER /
# AGEZT_MODEL / AGEZT_ALLOW_ALL / AGEZT_VAULT_PASSPHRASE pass through as env.
# The vault encrypts itself with the machine-bound key (M934) - no passphrase
# needed unless .env sets one.

param(
    [switch]$Fresh,
    [switch]$SkipBuild,
    [switch]$Pull,
    [string]$WebAddr = "127.0.0.1:8899"
)

$ErrorActionPreference = "Stop"
$ScriptDir = $PSScriptRoot
$Root = Split-Path -Parent $ScriptDir
Set-Location $Root
$DevHome = Join-Path $Root ".dev-home"
$Agezt = Join-Path $Root "agezt.exe"
$Agt = Join-Path $Root "agt.exe"

function Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }

# --- locate .env: next to the script, else the main repo root (worktree case)
$EnvFile = Join-Path $Root ".env"
if (-not (Test-Path $EnvFile)) {
    $common = git -C $Root rev-parse --git-common-dir 2>$null
    if ($common) {
        if (-not [System.IO.Path]::IsPathRooted($common)) { $common = Join-Path $Root $common }
        if (Test-Path $common) {
            $mainRoot = Split-Path (Resolve-Path $common) -Parent
            if (Test-Path (Join-Path $mainRoot ".env")) { $EnvFile = Join-Path $mainRoot ".env" }
        }
    }
}

# --- parse .env (KEY=VALUE, # comments) without echoing secrets to the console
$DotEnv = @{}
if (Test-Path $EnvFile) {
    Step "loading $EnvFile"
    foreach ($line in Get-Content $EnvFile) {
        $t = $line.Trim()
        if ($t -eq "" -or $t.StartsWith("#")) { continue }
        $i = $t.IndexOf("=")
        if ($i -lt 1) { continue }
        $DotEnv[$t.Substring(0, $i).Trim()] = $t.Substring($i + 1).Trim().Trim('"')
    }
} else {
    Write-Host "    (no .env found - vault will be seeded empty; add keys via bin\agt.exe provider creds set)" -ForegroundColor Yellow
}

# --- isolated home
if ($Fresh -and (Test-Path $DevHome)) { Step "wiping $DevHome"; Remove-Item -Recurse -Force $DevHome }
New-Item -ItemType Directory -Force -Path $DevHome | Out-Null
$env:AGEZT_HOME = $DevHome

# pass-through env the daemon reads; everything else stays out of the process env
foreach ($k in @("AGEZT_PROVIDER", "AGEZT_MODEL", "AGEZT_ALLOW_ALL", "AGEZT_VAULT_PASSPHRASE")) {
    if ($DotEnv.ContainsKey($k)) { Set-Item -Path "env:$k" -Value $DotEnv[$k] }
}

# --- build freshness (M972): the daemon embeds the web UI at build time, so a
# stale checkout silently ships an OLD console (e.g. no Fallback Chains page,
# old agent edit form). Print the revision we're building and shout when the
# branch is behind its upstream, so a redeploy that "changed nothing" is
# obvious. Pass -Pull to fast-forward to the latest before building.
if (-not $SkipBuild) {
    $branch = (git -C $Root rev-parse --abbrev-ref HEAD 2>$null)
    if ($Pull) {
        Step "git pull --ff-only ($branch)"
        git -C $Root pull --ff-only
    }
    $head = (git -C $Root rev-parse --short HEAD 2>$null)
    if ($head) { Step "building $branch @ $head" }
    git -C $Root fetch -q 2>$null
    $behind = (git -C $Root rev-list --count "HEAD..@{upstream}" 2>$null)
    if ($behind -and ([int]$behind) -gt 0) {
        Write-Host ("    WARNING: this checkout is {0} commit(s) BEHIND its upstream - the build and its embedded console will be OLD. Run:  git pull   (or .\dev.ps1 -Pull)" -f $behind) -ForegroundColor Yellow
    }
}

# --- build
if (-not $SkipBuild) {
    Step "go build .\agezt.exe + .\agt.exe"
    go build -o $Agezt ./cmd/agezt; if ($LASTEXITCODE -ne 0) { exit 1 }
    go build -o $Agt ./cmd/agt;   if ($LASTEXITCODE -ne 0) { exit 1 }
}

# --- seed catalog (once): copy the real synced catalog read-only, else sync offline
$DevCatalog = Join-Path $DevHome "catalog"
if (-not (Test-Path $DevCatalog)) {
    $RealCatalog = Join-Path $HOME ".agezt\catalog"
    if (Test-Path $RealCatalog) {
        Step "seeding catalog (copy of ~/.agezt/catalog, read-only source)"
        Copy-Item -Recurse $RealCatalog $DevCatalog
    } else {
        Step "seeding catalog (agt catalog sync --local)"
        & $Agt catalog sync --local | Out-Null  # best effort; needs network
        if ($LASTEXITCODE -ne 0) { Write-Host "    catalog sync failed - daemon boots on the mock; sync later from the console" -ForegroundColor Yellow }
    }
}

# --- seed vault defaults: every *_API_KEY from .env goes into the DEV vault
$keys = @($DotEnv.Keys | Where-Object { $_ -like "*_API_KEY" })
if ($keys.Count -gt 0) {
    Step ("seeding vault ({0} key(s): {1})" -f $keys.Count, ($keys -join ", "))
    foreach ($k in $keys) { & $Agt provider creds set "$k" "$($DotEnv[$k])" | Out-Null }
}

# --- run
$env:AGEZT_WEB_ADDR = $WebAddr
Step "starting agezt.exe  (home=$DevHome, console=http://$WebAddr, Ctrl+C to stop)"
& $Agezt
