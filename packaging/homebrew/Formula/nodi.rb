class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.4.1"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.4.1/nodi-darwin-arm64.zip"
      sha256 "81fe6e9a9bacf387c922f22a33fa40ba5796d0c1b5c1d96f4176a7a20724e997"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.4.1/nodi-darwin-amd64.zip"
      sha256 "2fa35281a08e5b73c5f736e1bac258f4d53304d80a3561fb76b4ab3b7ac5bee2"
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
