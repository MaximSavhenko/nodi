param([string]$Version = '0.3.0', [string]$BundleDirectory = 'dist', [string]$Go = 'go')
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows') { throw 'Upgrade smoke requires a disposable Windows CI runner.' }
$root = Join-Path $env:RUNNER_TEMP ('nodi-upgrade-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path "$root/config","$root/downloads","$root/bin",'.tools' -Force | Out-Null
$savedConfig = $env:NODI_CONFIG_DIR
$savedRead = $env:NODI_READ_TOKEN
$savedWrite = $env:NODI_WRITE_TOKEN
try {
    $env:NODI_CONFIG_DIR = "$root/config"
    $env:NODI_READ_TOKEN = $null; $env:NODI_WRITE_TOKEN = $null
    # The protected credential format is unchanged from 0.2.1. These are fictional
    # values, never sent to GitHub; validate decoding as well as byte preservation.
    @'
package main
import("fmt";"nodi/internal/config";"os")
func main(){
 if len(os.Args)>1 && os.Args[1]=="save" {
  must(config.Save(config.Config{Repository:"owner/private"}))
  must(config.SaveToken("fictional-upgrade-read",false))
  must(config.SaveToken("fictional-upgrade-write",true))
 } else {
  c,e:=config.Load();must(e);if c.Repository!="owner/private"{panic("repository changed")}
  for _,w:=range []bool{false,true}{t,e:=config.Token(w);must(e);expected:="fictional-upgrade-read";if w{expected="fictional-upgrade-write"};if t!=expected{panic("credential changed")}}
  fmt.Println("PASS: both saved credential roles decode after upgrade")
 }
}
func must(e error){if e!=nil{panic(e)}}
'@ | Set-Content -LiteralPath .tools/upgrade-proof.go -Encoding utf8NoBOM
    & $Go run .tools/upgrade-proof.go save
    if ($LASTEXITCODE -ne 0) { throw 'Credential fixture creation failed' }
    [IO.File]::WriteAllText("$root/downloads/user-edited.step", 'preserved recipient file')
    & "$PSScriptRoot/install-windows.ps1" -Repository MaximSavhenko/nodi -Release v0.2.1 -InstallDirectory "$root/bin" -NoPath -NoCompletion
    if ($LASTEXITCODE -ne 0 -or (& "$root/bin/nodi.exe" version) -cne 'nodi 0.2.1 windows/amd64') { throw 'Old installation failed' }
    $before = @{}
    foreach ($file in Get-ChildItem -LiteralPath "$root/config","$root/downloads" -Recurse -File) {
        $before[$file.FullName] = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
    }
    & "$PSScriptRoot/install-windows.ps1" -BundleDirectory $BundleDirectory -InstallDirectory "$root/bin" -NoPath -NoCompletion
    if ($LASTEXITCODE -ne 0 -or (& "$root/bin/nodi.exe" version) -cne "nodi $Version windows/amd64") { throw 'Upgrade failed' }
    foreach ($name in $before.Keys) {
        if ((Get-FileHash -LiteralPath $name -Algorithm SHA256).Hash -cne $before[$name]) { throw 'Upgrade changed settings, credentials or downloaded files' }
    }
    & $Go run .tools/upgrade-proof.go
    if ($LASTEXITCODE -ne 0) { throw 'Saved credential decoding failed' }
    & "$root/bin/nodi.exe" help
    if ($LASTEXITCODE -ne 0) { throw 'Upgraded help failed' }
    Write-Output "PASS: real Windows installer upgrade 0.2.1 -> $Version, settings/credentials/download preservation"
} finally {
    $env:NODI_CONFIG_DIR = $savedConfig; $env:NODI_READ_TOKEN = $savedRead; $env:NODI_WRITE_TOKEN = $savedWrite
    # Keep the disposable runner's evidence; do not clean user data or profiles.
}
