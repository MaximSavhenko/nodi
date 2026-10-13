class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.5.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-arm64.zip"
      sha256 "d3a9a9e45da6d1de11eab50f318973805e25996d95754fff2d5b2b120f660aea"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-amd64.zip"
      sha256 "9749eb48a75023aeb2d158b95011edd5bbdc9c24efbeac01ebc1bb839d2862bf"
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
