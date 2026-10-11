[CmdletBinding(DefaultParameterSetName = 'Local')]
param(
    [Parameter(Mandatory, ParameterSetName = 'Local')][string]$BundleDirectory,
    [Parameter(Mandatory, ParameterSetName = 'Remote')][ValidatePattern('^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$')][string]$Repository,
    [Parameter(Mandatory, ParameterSetName = 'Remote')][ValidatePattern('^v\d+\.\d+\.\d+$')][string]$Release,
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA 'Nodi\bin'),
    [switch]$NoPath,
    [switch]$NoCompletion
)
$ErrorActionPreference = 'Stop'
if (-not [Environment]::Is64BitOperatingSystem -or $env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { throw 'This installer requires Windows x64.' }
$assetName = 'nodi-windows-amd64.exe'
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('nodi-install-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
    if ($PSCmdlet.ParameterSetName -eq 'Remote') {
        # Only a user-selected GitHub source and an explicit release are trusted.
        # Token is read from the environment, never from command arguments.
        $headers = @{ 'Accept' = 'application/vnd.github+json'; 'User-Agent' = 'nodi-installer'; 'X-GitHub-Api-Version' = '2022-11-28' }
        if ($env:NODI_INSTALL_TOKEN) { $headers.Authorization = 'Bearer ' + $env:NODI_INSTALL_TOKEN }
        try { $remote = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/tags/$Release" -Headers $headers } catch { throw 'Cannot read trusted GitHub release. Check repository, release and access.' }
        if ($remote.draft -or $remote.prerelease -or $remote.tag_name -ne $Release) { throw 'Expected a published stable CLI release.' }
        foreach ($name in @($assetName, 'SHA256SUMS')) {
            $assets = @($remote.assets | Where-Object { $_.name -ceq $name -and $_.state -eq 'uploaded' })
            if ($assets.Count -ne 1 -or $assets[0].size -gt 33554432) { throw "Missing or oversized asset: $name" }
            # Handle the API redirect explicitly; never forward Authorization to CDN.
            $request = [Net.HttpWebRequest]::Create("https://api.github.com/repos/$Repository/releases/assets/$($assets[0].id)")
            $request.AllowAutoRedirect = $false; $request.Method = 'GET'; $request.Accept = 'application/octet-stream'; $request.UserAgent = 'nodi-installer'
            $request.Headers['X-GitHub-Api-Version'] = '2022-11-28'
            if ($env:NODI_INSTALL_TOKEN) { $request.Headers['Authorization'] = 'Bearer ' + $env:NODI_INSTALL_TOKEN }
            try {
                $response = $request.GetResponse()
                if ([int]$response.StatusCode -eq 302 -or [int]$response.StatusCode -eq 301 -or [int]$response.StatusCode -eq 307) {
                    $redirect = [Uri]$response.Headers['Location']; $response.Close()
                    if ($redirect.Scheme -ne 'https') { throw 'HTTPS redirect required' }
                    $request = [Net.HttpWebRequest]::Create($redirect); $request.AllowAutoRedirect = $false
                    $response = $request.GetResponse()
                }
                $stream = $response.GetResponseStream(); $file = [IO.File]::Create((Join-Path $temporary $name))
                try {
                    $buffer = New-Object byte[] 65536; $total = 0
                    while (($count = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                        $total += $count; if ($total -gt 33554432) { throw 'Download exceeds 32 MiB' }; $file.Write($buffer, 0, $count)
                    }
                } finally { $file.Dispose(); $stream.Dispose(); $response.Close() }
            } catch { throw "Cannot download GitHub asset: $name" }
        }
    } else {
        Copy-Item -LiteralPath (Join-Path $BundleDirectory $assetName) -Destination $temporary
        Copy-Item -LiteralPath (Join-Path $BundleDirectory 'SHA256SUMS') -Destination $temporary
    }
    $matches = @(Get-Content -LiteralPath (Join-Path $temporary 'SHA256SUMS') | Where-Object { $_ -cmatch "^[0-9a-f]{64}  $([regex]::Escape($assetName))$" })
    if ($matches.Count -ne 1) { throw 'Missing or ambiguous checksum' }
    $expected = $matches[0].Substring(0, 64)
    $actual = (Get-FileHash -LiteralPath (Join-Path $temporary $assetName) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -cne $expected) { throw 'SHA-256 mismatch; installation refused' }
    New-Item -ItemType Directory -Path $InstallDirectory -Force | Out-Null
    $installed = Join-Path $InstallDirectory 'nodi.exe'
    Copy-Item -LiteralPath (Join-Path $temporary $assetName) -Destination $installed -Force
    & $installed version
    if ($LASTEXITCODE -ne 0) { throw 'Installed binary failed version check' }
    if (-not $NoPath) {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $parts = @($userPath -split ';' | Where-Object { $_ })
        if ($parts -notcontains $InstallDirectory) { [Environment]::SetEnvironmentVariable('Path', (($parts + $InstallDirectory) -join ';'), 'User') }
        if (($env:Path -split ';') -notcontains $InstallDirectory) { $env:Path += ';' + $InstallDirectory }
    }
    Write-Output "Installed: $installed"
    if (-not $NoCompletion) {
        # Old releases do not have setup; installing them must remain supported.
        $helpText = (& $installed help) -join "`n"
        if ($helpText -match 'nodi setup') {
            $nodiSetupArguments = @('setup')
            if ($helpText -match 'nodi setup --completion') { $nodiSetupArguments += '--completion' }
            & $installed @nodiSetupArguments
            if ($LASTEXITCODE -ne 0) { Write-Warning 'Completion setup did not finish. Run nodi setup --completion again; the binary is installed.' }
        } else {
            Write-Output 'This version requires manual completion setup. See nodi completion.'
        }
    }
} finally {
    # Only this invocation's verified, freshly created temporary directory.
    $resolvedTemporary = [IO.Path]::GetFullPath($temporary)
    $resolvedTempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolvedTemporary.StartsWith($resolvedTempRoot, [StringComparison]::OrdinalIgnoreCase) -or (Split-Path $resolvedTemporary -Leaf) -notmatch '^nodi-install-[0-9a-f]{32}$') { throw 'Unsafe cleanup path' }
    Remove-Item -LiteralPath $resolvedTemporary -Recurse -Force
}
