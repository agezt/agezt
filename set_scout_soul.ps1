$soul = Get-Content 'D:\Codebox\PROJECTS\AGEZT\scout_soul.txt' -Raw
$proc = Start-Process -FilePath "agt" -ArgumentList "agent","set","scout","--soul",$soul -NoNewWindow -Wait -PassThru
exit $proc.ExitCode
