$s = Get-Content "D:\Codebox\PROJECTS\AGEZT\scout_soul.txt" -Raw
$s = $s -replace "`r`n", " "
$s = $s -replace "`n", " "
$s = $s.Trim()
& "D:\Codebox\PROJECTS\AGEZT\agt.exe" agent set scout --soul $s
