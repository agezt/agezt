# SPDX-License-Identifier: MIT
#
# webui-e2e.ps1 - Windows/PowerShell harness for the embedded Web UI e2e gate.
# Boots a keyless demo daemon, seeds one run, extracts the tokenized Web UI URL,
# and drives the production go:embed SPA with Playwright.

[CmdletBinding()]
param(
  [string]$AgeztBin = "",
  [string]$AgtBin = "",
  [int]$Port = 18787
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
  Write-Error "WEBUI-E2E FAIL: $Message"
  exit 1
}

function Ok($Message) {
  Write-Host "  ok: $Message"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("agezt-webui-e2e-" + [System.Guid]::NewGuid().ToString("N"))
$ageztHome = Join-Path $tmp "home"
$outLog = Join-Path $ageztHome "daemon.out.log"
$errLog = Join-Path $ageztHome "daemon.err.log"
$proc = $null

function Read-E2ELogs {
  $txt = ""
  if (Test-Path $script:outLog) {
    $txt += Get-Content $script:outLog -Raw
  }
  $txt += "`n"
  if (Test-Path $script:errLog) {
    $txt += Get-Content $script:errLog -Raw
  }
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
    } finally {
      Pop-Location
    }
  }
  if (-not $AgtBin) {
    Fail "pass both -AgeztBin and -AgtBin, or pass neither so the harness can build them"
  }
  if (-not (Test-Path $AgeztBin)) {
    Fail "agezt binary not found: $AgeztBin"
  }
  if (-not (Test-Path $AgtBin)) {
    Fail "agt binary not found: $AgtBin"
  }

  & $AgtBin catalog sync --local | Out-Null

  Write-Host "starting daemon (demo echo, Web UI on :$Port)..."
  $env:AGEZT_DEMO_ECHO = "1"
  $env:AGEZT_WEB_ADDR = "127.0.0.1:$Port"
  $proc = Start-Process -FilePath $AgeztBin -RedirectStandardOutput $outLog -RedirectStandardError $errLog -PassThru -WindowStyle Hidden

  $ready = $false
  for ($i = 0; $i -lt 80; $i++) {
    if ((Read-E2ELogs) -match "daemon ready") {
      $ready = $true
      break
    }
    Start-Sleep -Milliseconds 250
  }
  if (-not $ready) {
    Fail "daemon did not become ready:`n$(Read-E2ELogs)"
  }
  Ok "daemon ready"

  $runOut = & $AgtBin run "hello e2e" -q 2>&1
  if (($runOut -join "`n") -notmatch "\[echo\]") {
    Fail "agt run did not echo: $($runOut -join ' ')"
  }
  Ok "seeded a run (hello e2e)"

  $url = ""
  $urlPattern = "http://127\.0\.0\.1:$Port/\?token=[a-f0-9]+"
  for ($i = 0; $i -lt 20; $i++) {
    $match = [regex]::Match((Read-E2ELogs), $urlPattern)
    if ($match.Success) {
      $url = $match.Value
      break
    }
    Start-Sleep -Milliseconds 250
  }
  if (-not $url) {
    Fail "could not find the Web UI URL in the daemon log:`n$(Read-E2ELogs)"
  }
  Ok "web ui url resolved"

  Write-Host "running Playwright against the embedded SPA..."
  Push-Location (Join-Path $repoRoot "frontend")
  try {
    $env:AGEZT_WEBUI_URL = $url
    npx playwright test
    if ($LASTEXITCODE -ne 0) {
      Fail "playwright e2e failed with exit code $LASTEXITCODE"
    }
  } finally {
    Pop-Location
    Remove-Item Env:AGEZT_WEBUI_URL -ErrorAction SilentlyContinue
  }
  Ok "Playwright e2e passed"
  Write-Host "WEBUI-E2E PASS"
} finally {
  if ($proc -and -not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
  }
  Remove-Item Env:AGEZT_DEMO_ECHO -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_WEB_ADDR -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_HOME -ErrorAction SilentlyContinue
  Remove-Item Env:GOMAXPROCS -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
