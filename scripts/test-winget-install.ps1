param([string]$Version = '0.4.0', [string]$Owner = 'MaximSavhenko')
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+$' -or $Owner -notmatch '^[A-Za-z0-9-]+$') { throw 'Invalid release identity.' }
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows') { throw 'This installation smoke is restricted to disposable Windows CI runners.' }
$manifestPath = Join-Path $env:RUNNER_TEMP 'nodi-winget-manifests'
Invoke-WebRequest -Uri "https://github.com/$Owner/nodi/releases/download/v$Version/winget-manifests.zip" -OutFile "$env:RUNNER_TEMP/nodi-manifests.zip"
Expand-Archive -LiteralPath "$env:RUNNER_TEMP/nodi-manifests.zip" -DestinationPath $manifestPath
winget validate --manifest $manifestPath
if ($LASTEXITCODE -ne 0) { throw 'Published manifest validation failed.' }
winget settings --enable LocalManifestFiles
if ($LASTEXITCODE -ne 0) { throw 'Runner cannot enable local manifest testing.' }
winget install --manifest $manifestPath --silent --disable-interactivity --accept-package-agreements --accept-source-agreements
if ($LASTEXITCODE -ne 0) { throw 'Portable winget installation failed.' }
$env:PATH = [Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')
$versionOutput = (nodi version).Trim()
if ($LASTEXITCODE -ne 0 -or $versionOutput -cne "nodi $Version windows/amd64") { throw 'Installed PATH alias/version check failed.' }
nodi help
if ($LASTEXITCODE -ne 0) { throw 'Installed help command failed.' }
# A local manifest is not correlated with the official source before acceptance.
# Verify its installed ARP name/version and uninstall using that same manifest.
$entries = foreach ($root in @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall','HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall')) {
    Get-ItemProperty "$root\*" -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -eq 'Nodi' -and $_.Publisher -eq $Owner }
}
if (@($entries).Count -ne 1 -or $entries.DisplayVersion -cne $Version) { throw 'Portable ARP version detection failed.' }
$entries | Select-Object DisplayName,DisplayVersion,Publisher,WinGetPackageIdentifier,WinGetSourceIdentifier
$listed = winget list --name Nodi --exact --accept-source-agreements --disable-interactivity | Out-String
Write-Output $listed
if ($LASTEXITCODE -ne 0 -or $listed -notmatch [regex]::Escape($Version)) { throw 'winget installed version detection failed.' }
winget uninstall --manifest $manifestPath --silent --disable-interactivity --accept-source-agreements
if ($LASTEXITCODE -ne 0) { throw 'winget uninstall failed.' }
if (Get-Command nodi -ErrorAction SilentlyContinue) { throw 'Portable PATH alias remains after uninstall.' }
Write-Output 'PASS: published package, SHA-256, PATH, version/help, ARP/list version detection and uninstall. Official source acceptance is a separate pending check.'
