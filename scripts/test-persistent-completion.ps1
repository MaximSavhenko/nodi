param(
    [Parameter(Mandatory)][string]$Binary,
    [string]$Shell = (Get-Process -Id $PID).Path,
    # Only on disposable CI runners: backs up/restores the real user profiles.
    [switch]$ConfigureProfiles
)
$ErrorActionPreference = 'Stop'
$Binary = (Resolve-Path -LiteralPath $Binary).Path
$Shell = (Resolve-Path -LiteralPath $Shell).Path
$root = Join-Path ([IO.Path]::GetTempPath()) ('nodi-startup-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path "$root/config/completion","$root/bundle","$root/bin" -Force | Out-Null
$saved = @{}
foreach ($name in @('NODI_CONFIG_DIR','NODI_REPO','NODI_READ_TOKEN','NODI_WRITE_TOKEN')) {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$profiles = @()
try {
    $env:NODI_CONFIG_DIR = "$root/config"; $env:NODI_REPO = 'owner/private'
    $env:NODI_READ_TOKEN = 'completion-test-read'; $env:NODI_WRITE_TOKEN = $null
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $key = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes("owner/private`0false`0$env:NODI_READ_TOKEN")))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
    $cache = @{schema=1;updated=[DateTime]::UtcNow.ToString('o');items=@(@{project='nodi-ux-1';version='0.1.0'},@{project='nodi-ux-1';version='0.1.1'})} | ConvertTo-Json -Depth 5
    [IO.File]::WriteAllText("$root/config/completion/$key.json", $cache, [Text.UTF8Encoding]::new($false))
    if ($ConfigureProfiles) {
        $documents = [Environment]::GetFolderPath('MyDocuments')
        foreach ($dir in @('WindowsPowerShell','PowerShell')) {
            $path = Join-Path $documents "$dir/profile.ps1"
            $exists = Test-Path -LiteralPath $path
            $profiles += @{Path=$path;Exists=$exists;Bytes=$(if($exists){[IO.File]::ReadAllBytes($path)}else{$null})}
            New-Item -ItemType Directory -Path (Split-Path $path -Parent) -Force | Out-Null
            [IO.File]::WriteAllText($path, "`$global:NodiUserSetting = 'preserved'`r`n", [Text.UTF8Encoding]::new($true))
        }
        Copy-Item -LiteralPath $Binary -Destination "$root/bundle/nodi-windows-amd64.exe"
        $hash = (Get-FileHash -LiteralPath $Binary -Algorithm SHA256).Hash.ToLowerInvariant()
        [IO.File]::WriteAllText("$root/bundle/SHA256SUMS", "$hash  nodi-windows-amd64.exe`n", [Text.UTF8Encoding]::new($false))
        & "$PSScriptRoot/install-windows.ps1" -BundleDirectory "$root/bundle" -InstallDirectory "$root/bin" -NoPath -NoCompletion
        if ($LASTEXITCODE -ne 0) { throw 'Fixture install failed' }
        "y`ny" | & "$root/bin/nodi.exe" setup
        if ($LASTEXITCODE -ne 0) { throw 'Setup failed' }
        $configured = @{}
        foreach ($profile in $profiles) { $configured[$profile.Path] = [Convert]::ToBase64String([IO.File]::ReadAllBytes($profile.Path)) }
        # Re-running the default installer calls setup and must not prompt or duplicate.
        & "$PSScriptRoot/install-windows.ps1" -BundleDirectory "$root/bundle" -InstallDirectory "$root/bin" -NoPath
        & "$root/bin/nodi.exe" setup --check
        foreach ($profile in $profiles) {
            if ([Convert]::ToBase64String([IO.File]::ReadAllBytes($profile.Path)) -cne $configured[$profile.Path]) { throw 'Installer/check changed configured profile' }
        }
    }
    $verify = @'
$ErrorActionPreference = 'Stop'
if ($env:NODI_STARTUP_EXPECT_SETTING -and $global:NodiUserSetting -cne 'preserved') { throw 'Existing profile setting was lost' }
foreach ($case in @(@('nodi pub',@('publish')),@('nodi se',@('setup')),@('nodi get nod',@('nodi-ux-1')),@('nodi get nodi-ux-1 --version 0.1.',@('0.1.0','0.1.1')))) {
    $actual = @((TabExpansion2 -inputScript $case[0] -cursorColumn $case[0].Length).CompletionMatches | ForEach-Object CompletionText)
    foreach ($expected in $case[1]) {
        if ($actual -notcontains $expected) {
            Write-Output "Fresh startup: $($PSVersionTable.PSVersion); PATH=$env:PATH"
            Get-Command nodi,ConvertTo-Json,Register-ArgumentCompleter | Select-Object Name,Source | Format-Table | Out-String | Write-Output
            $Error | Select-Object -First 5 | ForEach-Object { Write-Output $_.ToString() }
            nodi __complete pub
            $oldEncoding = $OutputEncoding
            try {
                $OutputEncoding=[Text.UTF8Encoding]::new($false)
                '["pub"]' | nodi __complete --json
                if (Get-Command python -ErrorAction SilentlyContinue) { '["pub"]' | python -c 'import sys;print(sys.stdin.buffer.read().hex())' }
                Write-Output "Console encoding: $([Console]::OutputEncoding.WebName); pipeline encoding: $($OutputEncoding.WebName)"
            } finally { $OutputEncoding=$oldEncoding }
            # A secondary manual probe is diagnostic only; the original startup failure still fails CI.
            $probe = (nodi completion powershell | Out-String).Replace('    try {', '    [Console]::Error.WriteLine("Completer called: " + $wordToComplete); try {').Replace('} catch { }', '} catch { [Console]::Error.WriteLine($_.Exception.ToString()) }')
            $probe = $probe.Replace('$savedEncoding=$OutputEncoding', '[Console]::Error.WriteLine("JSON request: " + $request + "; program: " + $program); $savedEncoding=$OutputEncoding')
            $probe = $probe.Replace('foreach ($candidate in $candidates)', '[Console]::Error.WriteLine("Candidates: " + ($candidates -join ",") + "; exit: " + $LASTEXITCODE); foreach ($candidate in $candidates)')
            Invoke-Expression $probe
            $manual = TabExpansion2 -inputScript $case[0] -cursorColumn $case[0].Length
            Write-Output "Diagnostic manual probe: $($manual.CompletionMatches.CompletionText)"
            throw "Missing $expected in fresh startup: $actual"
        }
    }
}
Write-Output "PASS: fresh $($PSVersionTable.PSVersion) process, automatic profile loading, commands/projects/versions. Profile: $($PROFILE.CurrentUserAllHosts)"
'@
    [IO.File]::WriteAllText("$root/verify.ps1", $verify, [Text.UTF8Encoding]::new($false))
    $oldExpectation = $env:NODI_STARTUP_EXPECT_SETTING
    try {
        if ($ConfigureProfiles) { $env:NODI_STARTUP_EXPECT_SETTING = '1' }
        # Deliberately no -NoProfile, no dot-sourcing, no manual completion activation.
        & $Shell -NoLogo -NonInteractive -File "$root/verify.ps1"
        if ($LASTEXITCODE -ne 0) { throw 'Fresh shell startup completion failed' }
    } finally { $env:NODI_STARTUP_EXPECT_SETTING = $oldExpectation }
} finally {
    foreach ($profile in $profiles) {
        if ($profile.Exists) { [IO.File]::WriteAllBytes($profile.Path, $profile.Bytes) }
        else { Remove-Item -LiteralPath $profile.Path -Force -ErrorAction SilentlyContinue }
        Get-ChildItem -LiteralPath (Split-Path $profile.Path -Parent) -Filter 'profile.ps1.nodi-backup-*' | Where-Object CreationTimeUtc -ge (Get-Item -LiteralPath $root).CreationTimeUtc | Remove-Item -Force
    }
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
    $absolute = [IO.Path]::GetFullPath($root)
    $tempPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $absolute.StartsWith($tempPrefix,[StringComparison]::OrdinalIgnoreCase) -or (Split-Path $absolute -Leaf) -notmatch '^nodi-startup-[0-9a-f]{32}$') { throw 'Unsafe cleanup path' }
    Remove-Item -LiteralPath $absolute -Recurse -Force
}
