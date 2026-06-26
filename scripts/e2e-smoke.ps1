# SPDX-License-Identifier: MIT
#
# e2e-smoke.ps1 - Windows/PowerShell harness for the core daemon e2e gate.
# Boots a keyless demo daemon, then exercises the control plane, OpenAI API,
# REST API, validation paths, halt/resume, journal verification, and shutdown.

[CmdletBinding()]
param(
  [string]$AgeztBin = "",
  [string]$AgtBin = "",
  [int]$OpenAIPort = 18799,
  [int]$RestPort = 18800
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
  Write-Error "E2E FAIL: $Message"
  exit 1
}

function Ok($Message) {
  Write-Host "  ok: $Message"
}

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

function Invoke-CurlText([string[]]$ArgsList) {
  $out = & curl.exe @ArgsList 2>&1
  if ($LASTEXITCODE -ne 0) {
    Fail "curl failed ($LASTEXITCODE): $($out -join ' ')"
  }
  return ($out -join "`n")
}

function Invoke-StatusCode([string[]]$ArgsList) {
  $out = & curl.exe "-s" "-o" "NUL" "-w" "%{http_code}" @ArgsList 2>&1
  if ($LASTEXITCODE -ne 0) {
    Fail "curl status failed ($LASTEXITCODE): $($out -join ' ')"
  }
  return ($out -join "").Trim()
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("agezt-e2e-" + [System.Guid]::NewGuid().ToString("N"))
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

  Write-Host "starting daemon (demo echo, OpenAI + REST APIs)..."
  $env:AGEZT_DEMO_ECHO = "1"
  $env:AGEZT_API_ADDR = "127.0.0.1:$OpenAIPort"
  $env:AGEZT_REST_ADDR = "127.0.0.1:$RestPort"
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
  Start-Sleep -Seconds 1
  Ok "daemon ready"

  $runOut = & $AgtBin run "smoke" -q 2>&1
  if (($runOut -join "`n") -notmatch "\[echo\]") {
    Fail "agt run did not echo: $($runOut -join ' ')"
  }
  Ok "agt run (control-plane loop)"

  $doctorOut = & $AgtBin doctor 2>&1
  if (($doctorOut -join "`n") -notmatch "hash chain verified") {
    Fail "doctor did not verify journal chain: $($doctorOut -join ' ')"
  }
  $verifyOut = & $AgtBin journal verify 2>&1
  if (($verifyOut -join "`n") -notmatch '"ok":\s*true') {
    Fail "journal verify did not return ok=true: $($verifyOut -join ' ')"
  }
  Ok "doctor + journal chain verified"

  $logs = Read-E2ELogs
  $oai = [regex]::Match($logs, "openai api.*Bearer ([a-f0-9]{64})").Groups[1].Value
  $rest = [regex]::Match($logs, "rest api.*Bearer ([a-f0-9]{64})").Groups[1].Value
  if (-not $oai) { Fail "could not find OpenAI API token in daemon logs:`n$logs" }
  if (-not $rest) { Fail "could not find REST API token in daemon logs:`n$logs" }

  $chat = "http://127.0.0.1:$OpenAIPort/v1/chat/completions"
  $runs = "http://127.0.0.1:$RestPort/api/v1/runs"

  $chatBody = '{"model":"mock","messages":[{"role":"user","content":"hi"}]}'
  $chatFile = Join-Path $tmp "chat.json"
  [System.IO.File]::WriteAllText($chatFile, $chatBody)
  $chatOut = Invoke-CurlText @("-sS", "-H", "Authorization: Bearer $oai", "-H", "Content-Type: application/json", "--data-binary", "@$chatFile", $chat)
  if ($chatOut -notmatch '"content":"\[echo\]') {
    Fail "openai chat did not echo: $chatOut"
  }
  Ok "openai /v1/chat/completions"

  $streamBody = '{"model":"mock","stream":true,"messages":[{"role":"user","content":"streamcheck"}]}'
  $streamFile = Join-Path $tmp "stream.json"
  [System.IO.File]::WriteAllText($streamFile, $streamBody)
  $streamOut = Invoke-CurlText @("-sSN", "-H", "Authorization: Bearer $oai", "-H", "Content-Type: application/json", "--data-binary", "@$streamFile", $chat)
  if ($streamOut -notmatch "\[echo\]" -or $streamOut -notmatch "streamcheck" -or $streamOut -notmatch "data: \[DONE\]") {
    Fail "streaming response was incomplete: $streamOut"
  }
  Ok "openai streaming carries content"

  $restBody = '{"intent":"rest smoke"}'
  $restFile = Join-Path $tmp "rest.json"
  [System.IO.File]::WriteAllText($restFile, $restBody)
  $restOut = Invoke-CurlText @("-sS", "-H", "Authorization: Bearer $rest", "-H", "Content-Type: application/json", "--data-binary", "@$restFile", $runs)
  if ($restOut -notmatch '"status":"completed"') {
    Fail "REST run did not complete: $restOut"
  }
  Ok "native REST /api/v1/runs"

  $emptyFile = Join-Path $tmp "empty.json"
  [System.IO.File]::WriteAllText($emptyFile, "{}")
  if ((Invoke-StatusCode @("-H", "Authorization: Bearer WRONG", "--data-binary", "@$emptyFile", $chat)) -ne "401") {
    Fail "bad auth did not return 401"
  }
  $badFile = Join-Path $tmp "bad.json"
  [System.IO.File]::WriteAllText($badFile, "{bad")
  if ((Invoke-StatusCode @("-H", "Authorization: Bearer $oai", "-H", "Content-Type: application/json", "--data-binary", "@$badFile", $chat)) -ne "400") {
    Fail "malformed JSON did not return 400"
  }
  $big = Join-Path $tmp "big.json"
  $writer = [System.IO.StreamWriter]::new($big, $false)
  try {
    $writer.Write('{"model":"mock","messages":[{"role":"user","content":"')
    $chunk = "a" * 1000000
    for ($i = 0; $i -lt 17; $i++) { $writer.Write($chunk) }
    $writer.Write('"}]}')
  } finally {
    $writer.Dispose()
  }
  if ((Invoke-StatusCode @("-H", "Authorization: Bearer $oai", "-H", "Content-Type: application/json", "--data-binary", "@$big", $chat)) -ne "413") {
    Fail "oversized body did not return 413"
  }
  Ok "error paths: 401 / 400 / 413"

  $jobs = @()
  for ($i = 1; $i -le 10; $i++) {
    $bodyFile = Join-Path $tmp "concurrent-$i.json"
    [System.IO.File]::WriteAllText($bodyFile, "{`"model`":`"mock`",`"messages`":[{`"role`":`"user`",`"content`":`"c$i`"}]}")
    $jobs += Start-Job -ScriptBlock {
      param($Token, $Url, $BodyFile)
      & curl.exe "-s" "-o" "NUL" "-w" "%{http_code}" "-H" "Authorization: Bearer $Token" "-H" "Content-Type: application/json" "--data-binary" "@$BodyFile" $Url
    } -ArgumentList $oai, $chat, $bodyFile
  }
  $codes = @()
  try {
    Wait-Job -Job $jobs | Out-Null
    $codes = Receive-Job -Job $jobs
  } finally {
    Remove-Job -Job $jobs -Force -ErrorAction SilentlyContinue
  }
  $badCodes = @($codes | Where-Object { "$_".Trim() -ne "200" })
  if ($badCodes.Count -gt 0 -or $codes.Count -ne 10) {
    Fail "concurrent runs not all 200: $($codes -join ', ')"
  }
  Ok "10 concurrent runs all 200"

  & $AgtBin halt --reason e2e | Out-Null
  $haltStdout = Join-Path $tmp "halted.out"
  $haltStderr = Join-Path $tmp "halted.err"
  $prevErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  try {
    & $AgtBin run "during halt" -q > $haltStdout 2> $haltStderr
  } finally {
    $ErrorActionPreference = $prevErrorActionPreference
  }
  $haltedOut = ""
  if (Test-Path $haltStdout) { $haltedOut += Get-Content $haltStdout -Raw }
  if (Test-Path $haltStderr) { $haltedOut += "`n" + (Get-Content $haltStderr -Raw) }
  if ($haltedOut -notmatch "halted") {
    Fail "halt did not refuse a run: $haltedOut"
  }
  & $AgtBin resume --reason e2e | Out-Null
  $resumedOut = & $AgtBin run "after resume" -q 2>&1
  if (($resumedOut -join "`n") -notmatch "\[echo\]") {
    Fail "resume did not restore runs: $($resumedOut -join ' ')"
  }
  Ok "halt -> refuse -> resume"

  & $AgtBin shutdown | Out-Null
  Start-Sleep -Seconds 1
  if (-not $proc.HasExited) {
    Fail "daemon did not exit on shutdown"
  }
  $proc = $null
  $finalLogs = Read-E2ELogs
  if ($finalLogs -match "(?i)panic|runtime error|nil pointer dereference") {
    Fail "panic in daemon logs:`n$finalLogs"
  }
  Ok "graceful shutdown, 0 panics"

  Write-Host "E2E SMOKE: PASS"
} finally {
  if ($proc -and -not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
  }
  Remove-Item Env:AGEZT_DEMO_ECHO -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_API_ADDR -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_REST_ADDR -ErrorAction SilentlyContinue
  Remove-Item Env:AGEZT_HOME -ErrorAction SilentlyContinue
  Remove-Item Env:GOMAXPROCS -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
