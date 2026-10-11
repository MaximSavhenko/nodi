class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.4.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.4.0/nodi-darwin-arm64.zip"
      sha256 "7b9e2bd8056e979a26181f483c5159ee33d4c60d6162a9d201e75d339c05f2a6"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.4.0/nodi-darwin-amd64.zip"
      sha256 "79448b58a8cecd75be909cbebf074497044ca32306e912b40333102754ba2451"
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
