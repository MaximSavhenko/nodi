param(
    [Parameter(Mandatory)][string]$Version,
    [string]$DistributionDirectory = (Join-Path (Split-Path $PSScriptRoot -Parent) 'dist'),
    [ValidateSet('windows-amd64','darwin-arm64','darwin-amd64')][string]$Platform,
    [switch]$SkipRuntime
)
$ErrorActionPreference = 'Stop'
$expectedAssets = @('nodi-darwin-amd64','nodi-darwin-amd64.zip','nodi-darwin-arm64','nodi-darwin-arm64.zip','nodi-windows-amd64.exe','nodi-windows-amd64.zip')
$checksumRows = @(Get-Content -LiteralPath (Join-Path $DistributionDirectory 'SHA256SUMS'))
if ($checksumRows.Count -ne $expectedAssets.Count) { throw 'Unexpected checksum manifest length.' }
$seenAssets = @{}
foreach ($row in $checksumRows) {
    $checksumMatch = [regex]::Match($row, '^([0-9a-f]{64})  ([A-Za-z0-9.-]+)$')
    if (-not $checksumMatch.Success) { throw 'Malformed checksum row.' }
    $hash = $checksumMatch.Groups[1].Value; $name = $checksumMatch.Groups[2].Value
    if ($name -notin $expectedAssets -or $seenAssets.ContainsKey($name)) { throw 'Unexpected or duplicate artifact.' }
    $seenAssets[$name] = $true
    $actual = (Get-FileHash -LiteralPath (Join-Path $DistributionDirectory $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -cne $hash) { throw "Artifact integrity check failed: $name" }
}
Add-Type -AssemblyName System.IO.Compression.FileSystem
foreach ($target in @(@('nodi-windows-amd64.exe','nodi.exe','nodi-windows-amd64.zip'),@('nodi-darwin-arm64','nodi-darwin-arm64','nodi-darwin-arm64.zip'),@('nodi-darwin-amd64','nodi-darwin-amd64','nodi-darwin-amd64.zip'))) {
    $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $DistributionDirectory $target[2]))
    try {
        if ($zip.Entries.Count -ne 5 -or -not $zip.GetEntry($target[1]) -or -not $zip.GetEntry('LICENSE')) { throw 'Unexpected release ZIP contents.' }
        foreach ($completionName in @('nodi.bash','_nodi','nodi.ps1')) {
            $entry=$zip.GetEntry("completion/$completionName")
            if (-not $entry) { throw 'Missing shell completion in release ZIP.' }
            $reader=[IO.StreamReader]::new($entry.Open())
            try { $contents=$reader.ReadToEnd() } finally { $reader.Dispose() }
            if ($contents -cne [IO.File]::ReadAllText((Join-Path (Split-Path $PSScriptRoot -Parent) "internal/cli/shells/$completionName"))) { throw 'Unexpected completion script contents.' }
        }
        $stream = $zip.GetEntry($target[1]).Open(); $sha = [Security.Cryptography.SHA256]::Create()
        try { $entryHash = ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose(); $stream.Dispose() }
        $binaryHash = (Get-FileHash -LiteralPath (Join-Path $DistributionDirectory $target[0]) -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($entryHash -cne $binaryHash) { throw 'Release ZIP does not contain the release binary.' }
    } finally { $zip.Dispose() }
}
$metadata = Get-Content -LiteralPath (Join-Path $DistributionDirectory 'build-metadata.json') -Raw | ConvertFrom-Json
if ($metadata.version -cne $Version -or $metadata.tag -cne "v$Version" -or $metadata.confirmed) { throw 'Invalid build metadata.' }
if (-not $SkipRuntime) {
    if (-not $Platform) { throw 'Runtime platform must be explicit.' }
    $binaryName = "nodi-$Platform"; if ($Platform -eq 'windows-amd64') { $binaryName += '.exe' }
    $binary = Join-Path $DistributionDirectory $binaryName
    $savedPath = $env:PATH
    try {
        if ($Platform -ne 'windows-amd64') { & /bin/chmod +x $binary }
        $env:PATH = ''
        $actualVersion = (& $binary version).Trim()
        if ($LASTEXITCODE -ne 0 -or $actualVersion -cne "nodi $Version $($Platform.Replace('-','/'))") { throw "Unexpected runtime version: $actualVersion" }
        foreach ($alias in @('-v','--version')) {
            if ((& $binary $alias).Trim() -cne $actualVersion -or $LASTEXITCODE -ne 0) { throw 'Runtime version alias failed.' }
        }
        $helpOutput = & $binary help
        if ($LASTEXITCODE -ne 0 -or ($helpOutput -join "`n") -notmatch 'nodi get') { throw 'Runtime help check failed.' }
    } finally { $env:PATH = $savedPath }
}
Write-Output 'Distribution checksums, ZIP contents, metadata and requested native runtime: PASS'
