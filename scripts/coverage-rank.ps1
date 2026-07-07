# coverage-rank.ps1 — run `go test -cover` on every package and print a sorted
# table of coverage percentages, lowest first.
#
# Usage:
#   .\scripts\coverage-rank.ps1                  # all packages
#   .\scripts\coverage-rank.ps1 .\kernel\...     # only kernel packages
param(
    [string]$Target = ".\..."
)

go test -cover $Target 2>$null `
| Select-String '^ok\s+' `
| ForEach-Object {
    $parts = $_ -split '\s+'
    $pkg = $parts[1] -replace '^github\.com/agezt/agezt/', ''
    $cov = $parts[4] -replace '%$' -replace ','
    [PSCustomObject]@{ Package = $pkg; Coverage = [double]$cov }
} `
| Sort-Object Coverage `
| Format-Table -Property @{L='COV';E={"{0:N1}%" -f $_.Coverage}}, Package -AutoSize
