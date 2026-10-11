class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.2.1"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.2.1/nodi-darwin-arm64.zip"
      sha256 "8be9257cfa45b215cdc1446927b8dee8ab87ca4ddafecc3526e8f4eb897d07f4"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.2.1/nodi-darwin-amd64.zip"
      sha256 "cac0417e360f293dc8698420e8d0d5fd58f1707c4d218ddd5333159407b55b39"
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
