# SPDX-License-Identifier: MIT
<#
Production installer/runner/updater for AGEZT on Windows PowerShell.

Usage:
  .\install.ps1 install                 # install deps if possible, build, install service when NSSM exists
  .\install.ps1 run                     # run daemon in foreground
  .\install.ps1 start|stop|restart      # manage service
  .\install.ps1 status                  # service + health status
  .\install.ps1 update                  # fetch/checkout, rebuild, reinstall, restart
  .\install.ps1 expose tailscale        # controlled external access helper
  .\install.ps1 expose cloudflare
  .\install.ps1 expose ngrok

Useful overrides:
  .\install.ps1 install -AgeztRef v1.0.0 -AgeztRestAddr 127.0.0.1:8787
  .\install.ps1 install -AgeztRef main -AllowUnpinnedRef  # explicit branch opt-in
  .\install.ps1 expose tailscale -AllowRemoteInstall       # explicit third-party installer opt-in
  $env:OPENAI_API_KEY = '...'

Notes:
  - This is production-oriented: it builds static release binaries plus the embedded Web UI.
  - For a managed Windows service, install NSSM first or let this script try winget/choco.
  - Keep AGEZT_REST_ADDR on loopback; use expose providers for controlled access.
#>

[CmdletBinding()]
param(
  [Parameter(Position = 0)]
  [ValidateSet('install', 'update', 'run', 'start', 'stop', 'restart', 'status', 'logs', 'expose', 'help')]
  [string]$Action = 'install',

  [Parameter(Position = 1)]
  [ValidateSet('', 'tailscale', 'cloudflare', 'cloudflared', 'ngrok')]
  [string]$Provider = '',

  [string]$AgeztRepo = $(if ($env:AGEZT_REPO) { $env:AGEZT_REPO } else { 'https://github.com/agezt/agezt.git' }),
  [string]$AgeztRef = $(if ($env:AGEZT_REF) { $env:AGEZT_REF } else { 'v1.0.0' }),
  [switch]$AllowUnpinnedRef,
  [switch]$AllowRemoteInstall,
  [string]$AgeztSrc = $(if ($env:AGEZT_SRC) { $env:AGEZT_SRC } else { 'C:\ProgramData\Agezt\src' }),
  [string]$AgeztPrefix = $(if ($env:AGEZT_PREFIX) { $env:AGEZT_PREFIX } else { 'C:\Program Files\Agezt' }),
  [string]$AgeztHome = $(if ($env:AGEZT_HOME) { $env:AGEZT_HOME } else { 'C:\ProgramData\Agezt\home' }),
  [string]$AgeztConfigDir = $(if ($env:AGEZT_CONFIG_DIR) { $env:AGEZT_CONFIG_DIR } else { 'C:\ProgramData\Agezt\config' }),
  [string]$AgeztRestAddr = $(if ($env:AGEZT_REST_ADDR) { $env:AGEZT_REST_ADDR } else { '127.0.0.1:8787' }),
  [string]$GoVersion = $(if ($env:GO_VERSION) { $env:GO_VERSION } else { '1.26.4' }),
  [int]$NodeMajor = $(if ($env:NODE_MAJOR) { [int]$env:NODE_MAJOR } else { 22 }),
  [string]$ServiceName = 'AGEZT'
)

$ErrorActionPreference = 'Stop'
$EnvFile = Join-Path $AgeztConfigDir 'agezt.env'
$AgeztCurrentDir = Join-Path $AgeztPrefix 'current'
$AgeztExe = Join-Path $AgeztCurrentDir 'agezt.exe'
$AgtExe = Join-Path $AgeztCurrentDir 'agt.exe'

function Write-Step([string]$Message) { Write-Host "==> $Message" -ForegroundColor Green }
function Write-Warn([string]$Message) { Write-Warning $Message }
function Fail([string]$Message) { throw $Message }

function Test-Admin {
  $id = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [Security.Principal.WindowsPrincipal]::new($id)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Require-Admin {
  if (-not (Test-Admin)) { Fail 'Run this command from an elevated PowerShell session.' }
}

function Have([string]$Command) {
  return [bool](Get-Command $Command -ErrorAction SilentlyContinue)
}

function Ensure-Dir([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path)) {
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
  }
}

function Test-PinnedRef {
  return ($AgeztRef -match '^v\d' -or $AgeztRef -like 'refs/tags/*')
}

function Require-PinnedRef {
  if (Test-PinnedRef) { return }
  if (-not $AllowUnpinnedRef) {
    Fail "AgeztRef=$AgeztRef is not a pinned release tag; use -AgeztRef vX.Y.Z or pass -AllowUnpinnedRef to opt into branch installs."
  }
}

function Require-RemoteInstallOptIn {
  if (-not $AllowRemoteInstall) {
    Fail 'third-party package installation requires -AllowRemoteInstall; preinstall the dependency or opt in explicitly.'
  }
}

function Get-EnvFilePairs {
  $pairs = @()
  if (-not (Test-Path $EnvFile)) { return $pairs }
  foreach ($line in Get-Content $EnvFile) {
    if ($line -match '^\s*#' -or $line.Trim() -eq '') { continue }
    $idx = $line.IndexOf('=')
    if ($idx -lt 1) { continue }
    $name = $line.Substring(0, $idx).Trim()
    if ($name -match '(?i)(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL)') { continue }
    $value = $line.Substring($idx + 1).Trim()
    $pairs += "$name=$value"
  }
  return $pairs
}

function Install-WithPackageManager([string]$WingetId, [string]$ChocoName) {
  Require-RemoteInstallOptIn
  if (Have winget) {
    winget install --id $WingetId --exact --accept-source-agreements --accept-package-agreements
    return
  }
  if (Have choco) {
    choco install -y $ChocoName
    return
  }
  Fail "Neither winget nor choco is available. Install $WingetId manually, then rerun."
}

function Ensure-Prereqs {
  Require-Admin
  if (-not (Have git)) {
    Write-Step 'Installing Git'
    Install-WithPackageManager 'Git.Git' 'git'
  }
  if (-not (Have go)) {
    Require-RemoteInstallOptIn
    Write-Step "Installing Go $GoVersion"
    if (Have winget) {
      winget install --id GoLang.Go --exact --accept-source-agreements --accept-package-agreements
    } elseif (Have choco) {
      choco install -y golang --version $GoVersion
    } else {
      Fail "Install Go $GoVersion or newer manually, then rerun."
    }
  }
  if (-not (Have node)) {
    Write-Step "Installing Node.js $NodeMajor"
    Install-WithPackageManager 'OpenJS.NodeJS.LTS' 'nodejs-lts'
  } else {
    $nodeVersion = (& node -v).TrimStart('v')
    $major = [int]($nodeVersion.Split('.')[0])
    if ($major -lt $NodeMajor) { Write-Warn "Node.js $nodeVersion found; AGEZT expects Node.js $NodeMajor or newer." }
  }
  if (-not (Have npm)) { Fail 'npm is required and was not found.' }
}

function Sync-Source {
  Require-PinnedRef
  Ensure-Dir (Split-Path -Parent $AgeztSrc)
  $currentGoMod = Join-Path (Get-Location) 'go.mod'
  if (Test-Path (Join-Path $AgeztSrc '.git')) {
    Write-Step "Updating source in $AgeztSrc"
    git -C $AgeztSrc fetch --tags origin
    git -C $AgeztSrc checkout $AgeztRef
    if (-not (Test-PinnedRef)) {
      git -C $AgeztSrc pull --ff-only origin $AgeztRef
    }
    return
  }
  if ((Test-Path $currentGoMod) -and ((Get-Content $currentGoMod -Raw) -match '^module github\.com/agezt/agezt')) {
    Write-Step "Using current checkout: $(Get-Location)"
    if ((Resolve-Path (Get-Location)).Path -ne (Resolve-Path (Split-Path -Parent $AgeztSrc) -ErrorAction SilentlyContinue).Path) {
      if (Test-Path $AgeztSrc) { Remove-Item -Recurse -Force $AgeztSrc }
      git clone (Get-Location).Path $AgeztSrc
      git -C $AgeztSrc checkout $AgeztRef 2>$null
    }
    return
  }
  Write-Step "Cloning $AgeztRepo#$AgeztRef to $AgeztSrc"
  git clone --branch $AgeztRef $AgeztRepo $AgeztSrc
}

function Build-Agezt {
  Write-Step 'Building production frontend'
  Push-Location (Join-Path $AgeztSrc 'frontend')
  try {
    if (Test-Path 'package-lock.json') { npm ci } else { npm install }
    npm run build
  } finally {
    Pop-Location
  }

  Write-Step 'Building production Go binaries'
  Ensure-Dir $AgeztPrefix
  Ensure-Dir (Join-Path $AgeztPrefix 'releases')
  Push-Location $AgeztSrc
  try {
    $env:CGO_ENABLED = '0'
    $version = if ($env:VERSION) { $env:VERSION } else { (git describe --tags --always --dirty=-dev 2>$null) }
    if (-not $version) { $version = 'dev' }
    $commit = if ($env:COMMIT) { $env:COMMIT } else { (git rev-parse --short HEAD 2>$null) }
    if (-not $commit) { $commit = 'unknown' }
    $buildTime = if ($env:BUILD_TIME) { $env:BUILD_TIME } else { (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ') }
    $safeBuildTime = $buildTime -replace '[:]', ''
    $releaseDir = Join-Path (Join-Path $AgeztPrefix 'releases') "$version-$commit-$safeBuildTime"
    if (Test-Path $releaseDir) { Remove-Item -Recurse -Force $releaseDir }
    Ensure-Dir $releaseDir
    $stageAgeztExe = Join-Path $releaseDir 'agezt.exe'
    $stageAgtExe = Join-Path $releaseDir 'agt.exe'
    $ldflags = "-s -w -X github.com/agezt/agezt/internal/brand.Version=$version -X github.com/agezt/agezt/internal/brand.BuildCommit=$commit -X github.com/agezt/agezt/internal/brand.BuildTime=$buildTime"
    go mod download
    go build -trimpath -ldflags $ldflags -o $stageAgeztExe ./cmd/agezt
    go build -trimpath -ldflags $ldflags -o $stageAgtExe ./cmd/agt
    if (-not (Test-Path $stageAgeztExe) -or -not (Test-Path $stageAgtExe)) {
      Fail "staged AGEZT binaries were not created in $releaseDir"
    }
    return $releaseDir
  } finally {
    Pop-Location
  }
}

function Publish-AgeztRelease([string]$ReleaseDir) {
  if (-not (Test-Path (Join-Path $ReleaseDir 'agezt.exe')) -or -not (Test-Path (Join-Path $ReleaseDir 'agt.exe'))) {
    Fail "release directory is missing expected binaries: $ReleaseDir"
  }
  $nextDir = "$AgeztCurrentDir.next"
  $previousDir = "$AgeztCurrentDir.previous"
  if (Test-Path $nextDir) { Remove-Item -Recurse -Force $nextDir }
  Copy-Item -Recurse -Path $ReleaseDir -Destination $nextDir
  if (-not (Test-Path (Join-Path $nextDir 'agezt.exe')) -or -not (Test-Path (Join-Path $nextDir 'agt.exe'))) {
    Fail "prepared current.next directory is missing expected binaries: $nextDir"
  }
  if (Test-Path $previousDir) { Remove-Item -Recurse -Force $previousDir }
  if (Test-Path $AgeztCurrentDir) { Rename-Item -Path $AgeztCurrentDir -NewName (Split-Path -Leaf $previousDir) }
  try {
    Rename-Item -Path $nextDir -NewName (Split-Path -Leaf $AgeztCurrentDir)
  } catch {
    if ((Test-Path $previousDir) -and -not (Test-Path $AgeztCurrentDir)) {
      Rename-Item -Path $previousDir -NewName (Split-Path -Leaf $AgeztCurrentDir)
    }
    throw
  }
}

function Write-EnvFile {
  Write-Step "Writing $EnvFile"
  Ensure-Dir $AgeztHome
  Ensure-Dir $AgeztConfigDir
  if (-not (Test-Path $EnvFile)) {
    @"
# AGEZT production environment.
# Keep service bindings loopback by default. Use .\install.ps1 expose <provider>
# for controlled external access instead of binding directly to 0.0.0.0.
AGEZT_HOME=$AgeztHome
AGEZT_REST_ADDR=$AgeztRestAddr
# Optional tunnel/external access defaults. Prefer configuring these in the
# Web UI Config Center after first boot; changes require restart.
# AGEZT_TUNNEL=cloudflare
# AGEZT_TUNNEL_TARGET=http://127.0.0.1:8787
# AGEZT_TUNNEL_CMD=
# AGEZT_TUNNEL_NOTES=

# Optional provider defaults. Prefer configuring credentials through the Web UI
# setup screen or: agt provider creds set <provider>
# AGEZT_PROVIDER=openai
# AGEZT_MODEL=gpt-4.1
# OPENAI_API_KEY=
"@ | Set-Content -Encoding UTF8 -Path $EnvFile
    return
  }
  $content = Get-Content -Raw -Path $EnvFile
  if ($content -notmatch '(?m)^AGEZT_HOME=') { Add-Content -Path $EnvFile -Value "AGEZT_HOME=$AgeztHome" }
  if ($content -notmatch '(?m)^AGEZT_REST_ADDR=') { Add-Content -Path $EnvFile -Value "AGEZT_REST_ADDR=$AgeztRestAddr" }
}

function Ensure-Nssm {
  if (Have nssm) { return $true }
  Write-Warn 'NSSM is not installed; attempting package-manager install for Windows service support.'
  try {
    if (Have winget) { winget install --id NSSM.NSSM --exact --accept-source-agreements --accept-package-agreements; return (Have nssm) }
    if (Have choco) { choco install -y nssm; return (Have nssm) }
  } catch {
    Write-Warn $_.Exception.Message
  }
  return $false
}

function Install-Service {
  Require-Admin
  Write-EnvFile
  if (-not (Ensure-Nssm)) {
    Write-Warn 'NSSM unavailable. Binaries are installed; use .\install.ps1 run or install NSSM and rerun install.'
    return
  }
  $existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($existing) {
    Write-Step "Updating service $ServiceName"
    nssm set $ServiceName Application $AgeztExe | Out-Null
    nssm set $ServiceName AppParameters 'daemon' | Out-Null
    nssm set $ServiceName AppDirectory $AgeztHome | Out-Null
  } else {
    Write-Step "Installing service $ServiceName"
    nssm install $ServiceName $AgeztExe daemon | Out-Null
    nssm set $ServiceName AppDirectory $AgeztHome | Out-Null
  }
  $serviceEnv = @(Get-EnvFilePairs)
  if ($serviceEnv.Count -gt 0) {
    nssm set $ServiceName AppEnvironmentExtra @serviceEnv | Out-Null
  }
  nssm set $ServiceName Start SERVICE_AUTO_START | Out-Null
  nssm set $ServiceName AppStdout (Join-Path $AgeztHome 'agezt.stdout.log') | Out-Null
  nssm set $ServiceName AppStderr (Join-Path $AgeztHome 'agezt.stderr.log') | Out-Null
  nssm set $ServiceName AppRotateFiles 1 | Out-Null
  nssm set $ServiceName AppRotateOnline 1 | Out-Null
  nssm set $ServiceName AppRotateBytes 10485760 | Out-Null
}

function Install-All {
  Require-Admin
  Ensure-Prereqs
  Sync-Source
  $releaseDir = Build-Agezt
  $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($svc -and $svc.Status -ne 'Stopped') { Stop-Service -Name $ServiceName -Force }
  try {
    Publish-AgeztRelease $releaseDir
  } catch {
    if ($svc) { Start-Service -Name $ServiceName -ErrorAction SilentlyContinue }
    throw
  }
  Install-Service
  if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Start-Service -Name $ServiceName -ErrorAction SilentlyContinue
  }
  Write-Step 'AGEZT installed'
  Show-Status
  Write-Host "`nREST/Web binding: http://$AgeztRestAddr"
  Write-Host "Bearer token file: $AgeztHome\rest.token (created after daemon starts)"
  Write-Host "`nFor external access, prefer one of:"
  Write-Host "  .\install.ps1 expose tailscale"
  Write-Host "  .\install.ps1 expose cloudflare"
  Write-Host "  .\install.ps1 expose ngrok"
}

function Update-All {
  Require-Admin
  Ensure-Prereqs
  Sync-Source
  $releaseDir = Build-Agezt
  $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($svc -and $svc.Status -ne 'Stopped') { Stop-Service -Name $ServiceName -Force }
  try {
    Publish-AgeztRelease $releaseDir
  } catch {
    if ($svc) { Start-Service -Name $ServiceName -ErrorAction SilentlyContinue }
    throw
  }
  if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) { Start-Service -Name $ServiceName }
  Write-Step 'AGEZT updated'
  Show-Status
}

function Run-Foreground {
  Write-EnvFile
  $env:AGEZT_HOME = $AgeztHome
  $env:AGEZT_REST_ADDR = $AgeztRestAddr
  Write-Step 'Running AGEZT in foreground. Press Ctrl+C to stop.'
  & $AgeztExe daemon
}

function Show-Status {
  $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($svc) { $svc | Format-List Name,Status,StartType,ServiceType } else { Write-Warn "Service $ServiceName is not installed." }
  try {
    Invoke-WebRequest -UseBasicParsing -Uri "http://$AgeztRestAddr/healthz" -TimeoutSec 3 | Out-Null
    Write-Step "healthz OK at http://$AgeztRestAddr/healthz"
  } catch {
    Write-Warn "healthz not reachable at http://$AgeztRestAddr/healthz"
  }
  $tokenPath = Join-Path $AgeztHome 'rest.token'
  if (Test-Path $tokenPath) { Write-Step "REST token exists: $tokenPath" } else { Write-Warn "REST token not found yet: $tokenPath" }
}

function Show-Logs {
  $stdout = Join-Path $AgeztHome 'agezt.stdout.log'
  $stderr = Join-Path $AgeztHome 'agezt.stderr.log'
  if (Test-Path $stdout) { Get-Content -Wait -Tail 100 $stdout }
  elseif (Test-Path $stderr) { Get-Content -Wait -Tail 100 $stderr }
  else { Write-Warn 'No service logs found. If running foreground, logs are in the console.' }
}

function Install-Expose([string]$Name) {
  Require-Admin
  switch ($Name) {
    'tailscale' {
      if (-not (Have tailscale)) { Install-WithPackageManager 'Tailscale.Tailscale' 'tailscale' }
      Write-Host @"

Tailscale installed.
Controlled private access flow:
  1. tailscale up --ssh
  2. tailscale serve --bg --http=8787 http://$AgeztRestAddr
  3. Open http://<this-hostname>:8787 from devices in your tailnet.

Optional public Funnel, only if your tailnet policy allows it:
  tailscale funnel --bg 8787
"@
    }
    { $_ -in @('cloudflare', 'cloudflared') } {
      if (-not (Have cloudflared)) { Install-WithPackageManager 'Cloudflare.cloudflared' 'cloudflared' }
      if (-not (Ensure-Nssm)) { Fail 'NSSM is required to manage the Cloudflare quick tunnel service.' }
      $cloudflared = (Get-Command cloudflared).Source
      $tunnelService = 'AGEZT-Cloudflared'
      $existing = Get-Service -Name $tunnelService -ErrorAction SilentlyContinue
      if ($existing) {
        nssm set $tunnelService Application $cloudflared | Out-Null
        nssm set $tunnelService AppParameters "tunnel --no-autoupdate --url http://$AgeztRestAddr" | Out-Null
      } else {
        nssm install $tunnelService $cloudflared "tunnel --no-autoupdate --url http://$AgeztRestAddr" | Out-Null
      }
      nssm set $tunnelService AppDirectory $AgeztHome | Out-Null
      nssm set $tunnelService Start SERVICE_AUTO_START | Out-Null
      nssm set $tunnelService AppStdout (Join-Path $AgeztHome 'cloudflared.stdout.log') | Out-Null
      nssm set $tunnelService AppStderr (Join-Path $AgeztHome 'cloudflared.stderr.log') | Out-Null
      nssm set $tunnelService AppRotateFiles 1 | Out-Null
      nssm set $tunnelService AppRotateOnline 1 | Out-Null
      nssm set $tunnelService AppRotateBytes 10485760 | Out-Null
      Start-Service -Name $tunnelService -ErrorAction SilentlyContinue
      Write-Host @"

Cloudflare quick tunnel service installed and started.
Cloudflare will generate a random https://*.trycloudflare.com URL.

Show the generated URL:
  Get-Content -Tail 100 '$AgeztHome\cloudflared.stderr.log' | Select-String 'trycloudflare.com'
  Get-Content -Tail 100 '$AgeztHome\cloudflared.stdout.log' | Select-String 'trycloudflare.com'

Manage the tunnel service:
  Get-Service AGEZT-Cloudflared
  Restart-Service AGEZT-Cloudflared
  Stop-Service AGEZT-Cloudflared

Production note: trycloudflare.com quick tunnels are convenient but not stable
hostnames. For a stable hostname and Cloudflare Access policy, create a named
Cloudflare Tunnel and route your own domain.
"@
    }
    'ngrok' {
      if (-not (Have ngrok)) { Install-WithPackageManager 'Ngrok.Ngrok' 'ngrok' }
      Write-Host @"

ngrok installed.
Controlled access flow:
  1. ngrok config add-authtoken <token-from-ngrok-dashboard>
  2. ngrok http http://$AgeztRestAddr
  3. Restrict access in the ngrok dashboard with OAuth/IP policies when available.
"@
    }
    default { Fail 'Usage: .\install.ps1 expose tailscale|cloudflare|ngrok' }
  }
}

switch ($Action) {
  'install' { Install-All }
  'update' { Update-All }
  'run' { Run-Foreground }
  'start' { Require-Admin; Start-Service -Name $ServiceName }
  'stop' { Require-Admin; Stop-Service -Name $ServiceName }
  'restart' { Require-Admin; Restart-Service -Name $ServiceName }
  'status' { Show-Status }
  'logs' { Show-Logs }
  'expose' { Install-Expose $Provider }
  default {
    Write-Host @"
Usage: .\install.ps1 install|update|run|start|stop|restart|status|logs|expose

Examples:
  .\install.ps1 install
  .\install.ps1 update
  .\install.ps1 expose tailscale
"@
  }
}
