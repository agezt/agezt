# coverage-rank.ps1 — run `go test -cover` on every package and print a sorted
# table of coverage percentages, lowest first.
#
# Usage:
#   .\scripts\coverage-rank.ps1                  # all packages
#   .\scripts\coverage-rank.ps1 .\kernel\...     # only kernel packages
param(
    [string]$Target = ".\..."
)

$output = & go test -cover $Target 2>&1
$testExit = $LASTEXITCODE

$rows = foreach ($line in $output) {
    # Ignore packages that legitimately report "coverage: [no statements]".
    if ([string]$line -match '^ok\s+(\S+)\s+.*coverage:\s+([0-9]+(?:\.[0-9]+)?)%') {
        [PSCustomObject]@{
            Package  = $Matches[1] -replace '^github\.com/agezt/agezt/', ''
            Coverage = [double]::Parse(
                $Matches[2],
                [Globalization.CultureInfo]::InvariantCulture
            )
        }
    }
}

$rows `
| Sort-Object Coverage, Package `
| Format-Table -Property @{L='COV';E={"{0:N1}%" -f $_.Coverage}}, Package -AutoSize

if ($testExit -ne 0) {
    $output | Write-Error
    exit $testExit
}
