param(
    [string]$Version = '0.2.1',
    [string]$Owner = 'MaximSavhenko',
    [string]$License = 'MIT',
    [string]$DistributionDirectory = (Join-Path (Split-Path $PSScriptRoot -Parent) 'dist'),
    [string]$PackagingDirectory = (Join-Path (Split-Path $PSScriptRoot -Parent) 'packaging')
)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Only stable MAJOR.MINOR.PATCH versions are supported.' }
if ($Owner -notmatch '^[A-Za-z0-9][A-Za-z0-9-]*$') { throw 'Invalid GitHub owner.' }
if ($License -notin @('MIT','Proprietary')) { throw 'Unsupported license metadata.' }
$releaseBase = "https://github.com/$Owner/nodi/releases/download/v$Version"
$packageId = "$Owner.Nodi"
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
New-Item -ItemType Directory -Force (Join-Path $DistributionDirectory 'completion') | Out-Null
foreach ($completionName in @('nodi.bash','_nodi','nodi.ps1')) {
    Copy-Item -LiteralPath (Join-Path (Split-Path $PSScriptRoot -Parent) "internal/cli/shells/$completionName") -Destination (Join-Path $DistributionDirectory "completion/$completionName")
}
foreach ($target in @(@('nodi-windows-amd64.exe','nodi.exe','nodi-windows-amd64.zip'),@('nodi-darwin-arm64','nodi-darwin-arm64','nodi-darwin-arm64.zip'),@('nodi-darwin-amd64','nodi-darwin-amd64','nodi-darwin-amd64.zip'))) {
    $zipStream = [IO.File]::Open((Join-Path $DistributionDirectory $target[2]), [IO.FileMode]::Create, [IO.FileAccess]::ReadWrite)
    $zip = [IO.Compression.ZipArchive]::new($zipStream, [IO.Compression.ZipArchiveMode]::Create)
    try {
        $contents = @(@((Join-Path $DistributionDirectory $target[0]),$target[1]),@((Join-Path (Split-Path $PSScriptRoot -Parent) 'LICENSE'),'LICENSE'))
        foreach ($completionName in @('nodi.bash','_nodi','nodi.ps1')) { $contents += ,@((Join-Path $DistributionDirectory "completion/$completionName"),"completion/$completionName") }
        foreach ($content in $contents) {
            $entry = $zip.CreateEntry($content[1], [IO.Compression.CompressionLevel]::NoCompression)
            $entry.LastWriteTime = [DateTimeOffset]::new(1980,1,1,0,0,0,[TimeSpan]::Zero)
            $inputStream = [IO.File]::OpenRead($content[0]); $outputStream = $entry.Open()
            try { $inputStream.CopyTo($outputStream) } finally { $inputStream.Dispose(); $outputStream.Dispose() }
        }
    } finally { $zip.Dispose(); $zipStream.Dispose() }
}
$assetNames = @('nodi-darwin-amd64','nodi-darwin-amd64.zip','nodi-darwin-arm64','nodi-darwin-arm64.zip','nodi-windows-amd64.exe','nodi-windows-amd64.zip')
$assetHashes = @{}; $assetRecords = @()
foreach ($assetName in $assetNames) {
    $file = Get-Item -LiteralPath (Join-Path $DistributionDirectory $assetName)
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    $assetHashes[$assetName] = $hash
    $assetRecords += @{ name=$assetName; sha256=$hash; size=$file.Length }
}
$utf8 = [Text.UTF8Encoding]::new($false)
function Write-NodiText([string]$Path,[string]$Content) {
    New-Item -ItemType Directory -Path (Split-Path $Path -Parent) -Force | Out-Null
    [IO.File]::WriteAllText($Path, ($Content.Replace("`r`n","`n").TrimEnd()+"`n"), $utf8)
}
$checksumLines = foreach ($assetName in $assetNames) { '{0}  {1}' -f $assetHashes[$assetName], $assetName }
Write-NodiText (Join-Path $DistributionDirectory 'SHA256SUMS') ($checksumLines -join "`n")
$formulaLicense = 'license :cannot_represent'
if ($License -eq 'MIT') { $formulaLicense = 'license "MIT"' }
$formula = @"
class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/$Owner/nodi"
  version "$Version"
  $formulaLicense
  depends_on :macos

  on_macos do
    on_arm do
      url "$releaseBase/nodi-darwin-arm64.zip"
      sha256 "$($assetHashes['nodi-darwin-arm64.zip'])"
    end
    on_intel do
      url "$releaseBase/nodi-darwin-amd64.zip"
      sha256 "$($assetHashes['nodi-darwin-amd64.zip'])"
    end
  end

  def install
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    bin.install "nodi-darwin-#{arch}" => "nodi"
    chmod 0755, bin/"nodi"
    pkgshare.install "LICENSE"
    bash_completion.install "completion/nodi.bash" => "nodi"
    zsh_completion.install "completion/_nodi"
    pkgshare.install "completion/nodi.ps1"
  end

  test do
    assert_match "nodi #{version} darwin/", shell_output("#{bin}/nodi version")
    assert_match "nodi get", shell_output("#{bin}/nodi help")
  end
end
"@
Write-NodiText (Join-Path $PackagingDirectory 'homebrew\Formula\nodi.rb') $formula
Write-NodiText (Join-Path $DistributionDirectory 'nodi.rb') $formula
$manifestDirectory = Join-Path $PackagingDirectory "winget\manifests\m\$Owner\Nodi\$Version"
$schemaVersion = '1.10.0'
$versionManifest = @"
# yaml-language-server: `$schema=https://aka.ms/winget-manifest.version.1.10.0.schema.json
PackageIdentifier: $packageId
PackageVersion: $Version
DefaultLocale: en-US
ManifestType: version
ManifestVersion: $schemaVersion
"@
$localeManifest = @"
# yaml-language-server: `$schema=https://aka.ms/winget-manifest.defaultLocale.1.10.0.schema.json
PackageIdentifier: $packageId
PackageVersion: $Version
PackageLocale: en-US
Publisher: $Owner
PublisherUrl: https://github.com/$Owner
PublisherSupportUrl: https://github.com/$Owner/nodi/issues
PackageName: Nodi
PackageUrl: https://github.com/$Owner/nodi
License: $License
LicenseUrl: https://github.com/$Owner/nodi/blob/main/LICENSE
ShortDescription: Private engineering package transport through GitHub Releases
Moniker: nodi
Tags:
- cli
- cad
- engineering
ReleaseNotesUrl: https://github.com/$Owner/nodi/releases/tag/v$Version
ManifestType: defaultLocale
ManifestVersion: $schemaVersion
"@
$installerManifest = @"
# yaml-language-server: `$schema=https://aka.ms/winget-manifest.installer.1.10.0.schema.json
PackageIdentifier: $packageId
PackageVersion: $Version
MinimumOSVersion: 10.0.17763.0
InstallerType: zip
NestedInstallerType: portable
NestedInstallerFiles:
- RelativeFilePath: nodi.exe
  PortableCommandAlias: nodi
Commands:
- nodi
UpgradeBehavior: uninstallPrevious
Installers:
- Architecture: x64
  InstallerUrl: $releaseBase/nodi-windows-amd64.zip
  InstallerSha256: $($assetHashes['nodi-windows-amd64.zip'].ToUpperInvariant())
ManifestType: installer
ManifestVersion: $schemaVersion
"@
Write-NodiText (Join-Path $manifestDirectory "$packageId.yaml") $versionManifest
Write-NodiText (Join-Path $manifestDirectory "$packageId.locale.en-US.yaml") $localeManifest
Write-NodiText (Join-Path $manifestDirectory "$packageId.installer.yaml") $installerManifest
$manifestZip = Join-Path $DistributionDirectory 'winget-manifests.zip'
if (Test-Path -LiteralPath $manifestZip) { Remove-Item -LiteralPath $manifestZip }
[IO.Compression.ZipFile]::CreateFromDirectory($manifestDirectory, $manifestZip)
$commit = (& git -C (Split-Path $PSScriptRoot -Parent) rev-parse HEAD).Trim()
$metadata = @{ schema=1; version=$Version; tag="v$Version"; repository="$Owner/nodi"; source_commit=$commit; confirmed=$false; assets=$assetRecords }
Write-NodiText (Join-Path $DistributionDirectory 'build-metadata.json') ($metadata | ConvertTo-Json -Depth 10)
Write-Output "Generated Homebrew formula, portable Windows ZIP and winget manifests for $packageId $Version."
