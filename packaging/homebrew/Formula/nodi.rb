class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.5.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-arm64.zip"
      sha256 "10c81f2f4fed14f15d7b7ceb9f972d5bedce030769a342308f7c7ad70527192f"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-amd64.zip"
      sha256 "aabff38322847462c7a20ff9384a2d7966b7b04f32f821249396b521bfb97ab0"
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
