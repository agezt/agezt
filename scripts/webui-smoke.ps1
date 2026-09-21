# webui-smoke.ps1 — focused smoke test for the API-key entry refactor.
# Boots the same keyless demo daemon as webui-e2e.ps1 but runs only the
# `e2e/api-keys.spec.ts` Playwright spec — proves the refactored pages mount
# and have no console errors, in a couple of minutes instead of ten.

[CmdletBinding()]
param(
  [string]$AgeztBin = "",
  [string]$AgtBin = "",
  [int]$Port = 18789
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
  Write-Error "WEBUI-SMOKE FAIL: $Message"
  exit 1
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("agezt-webui-smoke-" + [System.Guid]::NewGuid().ToString("N"))
$ageztHome = Join-Path $tmp "home"
$outLog = Join-Path $ageztHome "daemon.out.log"
$errLog = Join-Path $ageztHome "daemon.err.log"
$proc = $null

function Read-DaemonLogs {
  $txt = ""
  if (Test-Path $script:outLog) { $txt += Get-Content $script:outLog -Raw }
  $txt += "`n"
  if (Test-Path $script:errLog) { $txt += Get-Content $script:errLog -Raw }
  return $txt
}

try {
  New-Item -ItemType Directory -Path $ageztHome -Force | Out-Null
  $env:AGEZT_HOME = $ageztHome
  $env:GOMAXPROCS = "3"

  if (-not $AgeztBin) {
    Write-Host "building binaries..."
    $AgeztBin = Join-Path $tmp "agezt.exe"
    $AgtBin = Join-Path $tmp "agt.exe"
    Push-Location $repoRoot
    try {
      go build -o $AgeztBin ./cmd/agezt
      go build -o $AgtBin ./cmd/agt
    } finally { Pop-Location }
  }
  if (-not $AgtBin) { Fail "pass both -AgeztBin and -AgtBin, or pass neither so the harness can build them" }
  if (-not (Test-Path $AgeztBin)) { Fail "agezt binary not found: $AgeztBin" }
  if (-not (Test-Path $AgtBin)) { Fail "agt binary not found: $AgtBin" }

  & $AgtBin catalog sync --local | Out-Null

  Write-Host "starting daemon (demo echo, Web UI on :$Port)..."
  $env:AGEZT_DEMO_ECHO = "1"
  $env:AGEZT_MODEL = "mock"
  $env:AGEZT_WEB_ADDR = "127.0.0.1:$Port"
  $proc = Start-Process -FilePath $AgeztBin -RedirectStandardOutput $outLog -RedirectStandardError $errLog -PassThru -WindowStyle Hidden

  $ready = $false
  for ($i = 0; $i -lt 80; $i++) {
    if ((Read-DaemonLogs) -match "daemon ready") { $ready = $true; break }
    Start-Sleep -Milliseconds 250
  }
  if (-not $ready) { Fail "daemon did not become ready:`n$(Read-DaemonLogs)" }
  Write-Host "  ok: daemon ready"

  $url = ""
  $urlPattern = "http://127\.0\.0\.1:$Port/\?token=[a-f0-9]+"
  for ($i = 0; $i -lt 20; $i++) {
    $match = [regex]::Match((Read-DaemonLogs), $urlPattern)
    if ($match.Success) { $url = $match.Value; break }
    Start-Sleep -Milliseconds 250
  }
  if (-not $url) { Fail "could not find Web UI URL:`n$(Read-DaemonLogs)" }
  Write-Host "  ok: web ui url resolved"

  Write-Host "running Playwright api-keys smoke spec..."
  Push-Location (Join-Path $repoRoot "frontend")
  try {
    $env:AGEZT_WEBUI_URL = $url
    npx playwright test e2e/api-keys.spec.ts --reporter=list
    if ($LASTEXITCODE -ne 0) { Fail "playwright smoke failed with exit code $LASTEXITCODE" }
  } finally {
    Pop-Location
    Remove-Item Env:AGEZT_WEBUI_URL -ErrorAction SilentlyContinue
  }
  Write-Host "WEBUI-SMOKE PASS"
} finally {
  if ($proc -and -not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
  }
  Remove-Item Env:AGEZT_DEMO_ECHO -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_MODEL -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_WEB_ADDR -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_HOME -ErrorAction SilentlyContinue
  Remove-Item Env:GOMAXPROCS -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
