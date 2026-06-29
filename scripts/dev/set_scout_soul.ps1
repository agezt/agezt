$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$soul = Get-Content (Join-Path $scriptDir "scout_soul.txt") -Raw
$agt = Join-Path $repoRoot "agt.exe"
if (!(Test-Path $agt)) {
  $agt = "agt"
}
$proc = Start-Process -FilePath $agt -ArgumentList "agent","set","scout","--soul",$soul -NoNewWindow -Wait -PassThru
exit $proc.ExitCode
