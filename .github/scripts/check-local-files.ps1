$ErrorActionPreference = 'Stop'
$repo = Join-Path $PSScriptRoot '../..'
$paths = @(git -C $repo -c core.quotePath=false ls-files --cached)
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$localPattern = '(^|/)(\.env(?:\.[^/]+)?|microsoft-account\.dat|skin-stations/[^/]+\.dat|launcher(?:-settings)?\.json|instances\.json|tasks\.json|\.lxmcl-[^/]*)$'
$localFiles = @($paths | Where-Object { $_ -cne '.env.example' -and $_ -match $localPattern })
if ($localFiles.Count -ne 0) {
    throw "Do not track local launcher configuration or credential files: $($localFiles -join ', ')"
}

$template = Get-Content -LiteralPath (Join-Path $repo '.env.example')
if ($template | Where-Object { $_ -notmatch '^\s*(?:#.*|[A-Z_][A-Z0-9_]*\s*=\s*)?$' }) {
    throw '.env.example must contain only comments and empty assignments.'
}
