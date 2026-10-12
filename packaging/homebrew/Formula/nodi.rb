class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.5.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-arm64.zip"
      sha256 "6af1a3316afadd182b528fc10bf9672bf9a4c0089e94b74801a3efa90efee5f7"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-amd64.zip"
      sha256 "7b1678b430e32bde1bbd3cb5010e1b8eb8215826485dceaea30897d084b6a3d8"
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
