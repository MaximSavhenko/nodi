param([Parameter(Mandatory)][string]$Binary)
$ErrorActionPreference='Stop'
$Binary=(Resolve-Path -LiteralPath $Binary).Path
$root=Join-Path ([IO.Path]::GetTempPath()) ('nodi-shell-smoke-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force "$root/bin","$root/config/completion","$root/fixture parts" | Out-Null
$windows=[Environment]::OSVersion.Platform -eq 'Win32NT'
$program=Join-Path "$root/bin" $(if($windows){'nodi.exe'}else{'nodi'})
Copy-Item -LiteralPath $Binary -Destination $program
if(-not $windows){ & /bin/chmod +x $program }
$saved=@{};foreach($name in @('PATH','NODI_CONFIG_DIR','NODI_REPO','NODI_READ_TOKEN','NODI_WRITE_TOKEN')){$saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location $root
try {
    $env:PATH="$root/bin"+[IO.Path]::PathSeparator+$env:PATH
    $env:NODI_CONFIG_DIR="$root/config"; $env:NODI_REPO='owner/private'
    $env:NODI_READ_TOKEN='completion-test-read';$env:NODI_WRITE_TOKEN='completion-test-write'
    foreach($role in @(@('false',$env:NODI_READ_TOKEN),@('true',$env:NODI_WRITE_TOKEN))) {
        $sha=[Security.Cryptography.SHA256]::Create()
        try { $key=([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes("owner/private`0$($role[0])`0$($role[1])")))).Replace('-','').ToLowerInvariant() } finally {$sha.Dispose()}
        $cache=@{schema=1;updated=[DateTime]::UtcNow.ToString('o');items=@(@{project='nodi-demo';version='0.1.0'},@{project='nodi-demo';version='0.1.1'})} | ConvertTo-Json -Depth 5
        [IO.File]::WriteAllText("$root/config/completion/$key.json",$cache,[Text.UTF8Encoding]::new($false))
    }
    nodi completion powershell | Out-String | Invoke-Expression
    foreach($case in @(@('nodi pub',@('publish')),@('nodi auth lo',@('login','logout')),@('nodi get nod',@('nodi-demo')),@('nodi get nodi-demo --version 0.1.',@('0.1.0','0.1.1')),@('nodi get nodi-demo --o',@('--output')),@('nodi publish --status r',@('released')))) {
        $result=TabExpansion2 -inputScript $case[0] -cursorColumn $case[0].Length
        $actual=@($result.CompletionMatches | ForEach-Object CompletionText)
        foreach($expected in $case[1]) {if($actual -notcontains $expected){throw "Missing completion $expected for $($case[0]): $actual"}}
    }
    $line='nodi publish nodi-demo fi'
    $result=TabExpansion2 -inputScript $line -cursorColumn $line.Length
    if(-not ($result.CompletionMatches | Where-Object {$_.ListItemText -eq 'fixture parts'})){throw 'Path completion with spaces failed.'}
    Write-Output 'PASS: real PowerShell native completer, commands/subcommands/flags/projects/versions/paths and multiple choices.'
} finally {
    Pop-Location
    foreach($name in $saved.Keys){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}
    $absolute=[IO.Path]::GetFullPath($root)
    $tempPrefix=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar
    if(-not $absolute.StartsWith($tempPrefix,[StringComparison]::OrdinalIgnoreCase) -or (Split-Path $absolute -Leaf) -notmatch '^nodi-shell-smoke-[0-9a-f]{32}$'){throw 'Unsafe smoke cleanup path.'}
    Remove-Item -LiteralPath $absolute -Recurse -Force
}
