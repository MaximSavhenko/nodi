param(
    [Parameter(Mandatory)][string]$Binary,
    [string]$Repository = 'MaximSavhenko/nodi-packages'
)
$ErrorActionPreference = 'Stop'
if ($Repository -cne 'MaximSavhenko/nodi-packages') { throw 'This integration test is restricted to the Nodi package repository.' }
if (-not $env:NODI_WRITE_TOKEN) { throw 'Provide an ephemeral write credential in NODI_WRITE_TOKEN. Never pass tokens as arguments.' }
if (-not $env:NODI_READ_TOKEN) { throw 'Provide a read credential in NODI_READ_TOKEN for the download check.' }
$binaryPath = (Resolve-Path -LiteralPath $Binary).Path
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('nodi06-test-' + [Guid]::NewGuid().ToString('N'))
$project = 'nodi06-delete-test-' + [DateTime]::UtcNow.ToString('yyyyMMddHHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
$savedRepo=$env:NODI_REPO; $savedConfig=$env:NODI_CONFIG_DIR
New-Item -ItemType Directory -Path $testRoot | Out-Null
$parent=Join-Path $testRoot 'parent'; $source=Join-Path $parent 'DXF'
New-Item -ItemType Directory -Path $source -Force | Out-Null
$file=Join-Path $source 'fixture.txt'
[IO.File]::WriteAllText($file,'NODI-06 isolated API fixture, not user CAD data.')
function Invoke-TestNodi {
    param([string[]]$Arguments,[string]$Confirmation)
    if ($Confirmation) { $text = $Confirmation | & $binaryPath @Arguments 2>&1 } else { $text = & $binaryPath @Arguments 2>&1 }
    if ($LASTEXITCODE -ne 0) { throw ('nodi failed: '+($text -join "`n")) }
    $text | Out-Host
    return ($text -join "`n")
}
try {
    $env:NODI_REPO=$Repository; $env:NODI_CONFIG_DIR=Join-Path $testRoot 'config'
    Push-Location $parent
    try { Invoke-TestNodi -Arguments @('init',('parent-'+[Guid]::NewGuid().ToString('N').Substring(0,8))) | Out-Null } finally { Pop-Location }
    $binding=Join-Path $parent '.nodi/project.json'; $originalBinding=(Get-FileHash -LiteralPath $binding).Hash
    Invoke-TestNodi -Arguments @('send',$source,'--name',$project,'--dry-run') | Out-Null
    Invoke-TestNodi -Arguments @('send',$source,'--name',$project) | Out-Null
    [IO.File]::WriteAllText($file,'Second isolated fixture revision.')
    Invoke-TestNodi -Arguments @('send',$source) | Out-Null
    $destination=Join-Path $testRoot 'download'
    Invoke-TestNodi -Arguments @('get',$project,'--version','0.1.1','--output',$destination) | Out-Null
    if ((Get-FileHash -LiteralPath $file).Hash -cne (Get-FileHash -LiteralPath (Join-Path $destination 'fixture.txt')).Hash) { throw 'Downloaded SHA-256 differs.' }
    $downloadHash=(Get-FileHash -LiteralPath (Join-Path $destination 'fixture.txt')).Hash
    Invoke-TestNodi -Arguments @('delete',$project,'--version','0.1.1','--dry-run') | Out-Null
    Invoke-TestNodi -Arguments @('delete',$project,'--version','0.1.1') -Confirmation $project | Out-Null
    $result=Invoke-TestNodi -Arguments @('send',$source)
    if ($result -notmatch 'Published .* 0\.1\.2 ') { throw 'Highest deleted version was reused.' }
    Invoke-TestNodi -Arguments @('delete',$project,'--all','--dry-run') | Out-Null
    Invoke-TestNodi -Arguments @('delete',$project,'--all') -Confirmation ('DELETE ALL '+$project) | Out-Null
    $result=Invoke-TestNodi -Arguments @('send',$source)
    if ($result -notmatch 'Published .* 0\.1\.3 ') { throw 'All-deleted history was reset.' }
    Invoke-TestNodi -Arguments @('delete',$project,'--all') -Confirmation ('DELETE ALL '+$project) | Out-Null
    if ($originalBinding -cne (Get-FileHash -LiteralPath $binding).Hash) { throw 'Parent binding changed.' }
    if ($downloadHash -cne (Get-FileHash -LiteralPath (Join-Path $destination 'fixture.txt')).Hash) { throw 'Deletion altered local download.' }
    Write-Output "PASS: isolated GitHub API test project $project. Releases and package tags removed; four permanent version-history refs retained. Local evidence: $testRoot"
} finally {
    $env:NODI_REPO=$savedRepo; $env:NODI_CONFIG_DIR=$savedConfig
    # Keep the small test folder as evidence. Never remove user directories.
}
