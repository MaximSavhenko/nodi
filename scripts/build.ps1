param(
    [string]$Go = 'go',
    [string]$Version = '0.2.1'
)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw 'Version must be MAJOR.MINOR.PATCH' }
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
$oldOS = $env:GOOS; $oldArch = $env:GOARCH; $oldCgo = $env:CGO_ENABLED
try {
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    $env:CGO_ENABLED = '0'
    foreach ($target in @(@('windows', 'amd64', 'nodi-windows-amd64.exe'), @('darwin', 'arm64', 'nodi-darwin-arm64'), @('darwin', 'amd64', 'nodi-darwin-amd64'))) {
        $env:GOOS = $target[0]; $env:GOARCH = $target[1]
        & $Go build -trimpath -buildvcs=false -ldflags "-s -w -buildid= -X main.version=$Version" -o (Join-Path 'dist' $target[2]) './cmd/nodi'
        if ($LASTEXITCODE -ne 0) { throw "Build failed: $($target[2])" }
    }
    $lines = Get-ChildItem -LiteralPath 'dist' -File | Where-Object { $_.Name -match '^nodi-(windows|darwin)-' } | Sort-Object Name | ForEach-Object {
        '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $_.Name
    }
    [IO.File]::WriteAllLines((Join-Path $projectRoot 'dist\SHA256SUMS'), [string[]]$lines, [Text.UTF8Encoding]::new($false))
} finally {
    $env:GOOS = $oldOS; $env:GOARCH = $oldArch; $env:CGO_ENABLED = $oldCgo
    Pop-Location
}
