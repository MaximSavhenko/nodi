class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.4.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.4.0/nodi-darwin-arm64.zip"
      sha256 "fffadd1d9226fd95225f9d531a36d059bbfe8d78cd6a411af8303bac84197ced"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.4.0/nodi-darwin-amd64.zip"
      sha256 "2c085aedff831e2100ca847a50dc33b763697aaf34a7a8afcb91f8d0d89fc1c4"
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
