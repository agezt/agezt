$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$s = Get-Content (Join-Path $scriptDir "scout_soul.txt") -Raw
$s = $s -replace "`r`n", " "
$s = $s -replace "`n", " "
$s = $s.Trim()
$agt = Join-Path $repoRoot "agt.exe"
if (Test-Path $agt) {
  & $agt agent set scout --soul $s
} else {
  & "agt" agent set scout --soul $s
}
