class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.2.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.2.0/nodi-darwin-arm64.zip"
      sha256 "bbf1f9da9b76ba8c700979f51b63c0ecdfda0a5b7fa22d309b5edeb6accb5483"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.2.0/nodi-darwin-amd64.zip"
      sha256 "1e8f6fcca0b35d33f0259444a166ce0f6ddfbd055c6dfb3f3820a8ff6ce630ae"
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
