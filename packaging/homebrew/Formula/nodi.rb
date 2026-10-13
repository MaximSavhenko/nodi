class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.5.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-arm64.zip"
      sha256 "a5c475bf8cbdcbdeb91750dbff987ddf9262046481aaffad5bbea4e1b1c171d2"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-amd64.zip"
      sha256 "f9b7c60b29444dbc0a9e1e7322457987956de8a75cb8dcad9946261516852152"
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
