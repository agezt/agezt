param(
    [switch]$Fresh,
    [switch]$SkipBuild,
    [switch]$Pull,
    [string]$WebAddr = "127.0.0.1:8899"
)

$ErrorActionPreference = "Stop"
$Script = Join-Path $PSScriptRoot "scripts\dev.ps1"
& $Script @PSBoundParameters
