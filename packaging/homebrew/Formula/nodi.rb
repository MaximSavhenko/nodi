class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.2.1"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.2.1/nodi-darwin-arm64.zip"
      sha256 "148bf117e962604340b8d1e758d6e255eb3721b48433f63dee21c3b32b483214"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.2.1/nodi-darwin-amd64.zip"
      sha256 "8d9ea1f6fa84dd4eb82120b799de3ddcc7620e493dc605ec198087986858a952"
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
