class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.3.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.3.0/nodi-darwin-arm64.zip"
      sha256 "38a18d965e3b0af4a86253f3f3e6c96dd80cae2aeabe1994ceefe799c9217496"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.3.0/nodi-darwin-amd64.zip"
      sha256 "23b41a49f621b27fd19f55ae93408cb037d37a3b0f638a1568ebe3f750143352"
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
