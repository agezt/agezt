# nav-screenshots.ps1 -” boots a keyless demo daemon and runs the
# nav-screenshots.spec.ts Playwright spec, which captures a PNG of every
# nav view. Outputs land in frontend/test-results/nav-screenshots/ so the
# operator can flip through them and verify each page renders real content.
#
# Differs from webui-smoke.ps1 / nav-audit.ps1 in that this one's job is
# "let me see with my own eyes". Run when somebody says "is page X really
# there?" -” point them at the screenshot.

[CmdletBinding()]
param(
  [string]$AgeztBin = "",
  [string]$AgtBin = "",
  [int]$Port = 18800
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
  Write-Error "NAV-SCREENSHOTS FAIL: $Message"
  exit 1
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("agezt-nav-shots-" + [System.Guid]::NewGuid().ToString("N"))
$ageztHome = Join-Path $tmp "home"
$outLog = Join-Path $ageztHome "daemon.out.log"
$errLog = Join-Path $ageztHome "daemon.err.log"
$proc = $null

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
  if (-not (Test-Path $AgeztBin)) { Fail "agezt binary not found: $AgeztBin" }
  if (-not (Test-Path $AgtBin)) { Fail "agt binary not found: $AgtBin" }

  & $AgtBin catalog sync --local | Out-Null

  Write-Host "starting daemon (demo echo, Web UI on :$Port)..."
  $env:AGEZT_DEMO_ECHO = "1"
  $env:AGEZT_MODEL = "mock"
  $env:AGEZT_WEB_ADDR = "127.0.0.1:$Port"
  $proc = Start-Process -FilePath $AgeztBin -RedirectStandardOutput $outLog -RedirectStandardError $errLog -PassThru -WindowStyle Hidden

  function Read-DaemonLogs {
    $txt = ""
    if (Test-Path $script:outLog) { $txt += Get-Content $script:outLog -Raw }
    $txt += "`n"
    if (Test-Path $script:errLog) { $txt += Get-Content $script:errLog -Raw }
    return $txt
  }

  $ready = $false
  for ($i = 0; $i -lt 80; $i++) {
    if ((Read-DaemonLogs) -match "daemon ready") { $ready = $true; break }
    Start-Sleep -Milliseconds 250
  }
  if (-not $ready) { Fail "daemon did not become ready" }
  Write-Host "  ok: daemon ready"

  $url = ""
  $urlPattern = "http://127\.0\.0\.1:$Port/\?token=[a-f0-9]+"
  for ($i = 0; $i -lt 20; $i++) {
    $match = [regex]::Match((Read-DaemonLogs), $urlPattern)
    if ($match.Success) { $url = $match.Value; break }
    Start-Sleep -Milliseconds 250
  }
  if (-not $url) { Fail "could not find Web UI URL" }
  Write-Host "  ok: web ui url resolved"

  Write-Host "running Playwright nav-screenshots spec (39 views)..."
  Push-Location (Join-Path $repoRoot "frontend")
  try {
    $env:AGEZT_WEBUI_URL = $url
    npx playwright test e2e/nav-screenshots.spec.ts --reporter=list
    if ($LASTEXITCODE -ne 0) { Fail "playwright failed with exit code $LASTEXITCODE" }
  } finally {
    Pop-Location
    Remove-Item Env:AGEZT_WEBUI_URL -ErrorAction SilentlyContinue
  }

  $shotDir = Join-Path $repoRoot "frontend/test-results/nav-screenshots"
  Write-Host "NAV-SCREENSHOTS PASS - $shotDir"
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
