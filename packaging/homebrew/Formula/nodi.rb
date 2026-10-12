class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.5.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-arm64.zip"
      sha256 "a2e121a87026418f81aa463db2038efed89971df6e8d9390b742079b0c5ff325"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.5.0/nodi-darwin-amd64.zip"
      sha256 "78dc92377587fb5723d6722f8ba134b160150a2b6c1ad120dd67f97d2578d2e5"
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
