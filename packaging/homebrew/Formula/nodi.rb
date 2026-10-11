class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.1.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.1.0/nodi-darwin-arm64.zip"
      sha256 "1d84dd85bde5d1b3d91addecb0660d2d0e50bd45ffbe1e428c414ec7587bd88d"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.1.0/nodi-darwin-amd64.zip"
      sha256 "e6007774114c8282aefeab7388241315a02ac7ff1ef4ea31758008937f4db754"
    end
  end

  def install
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    bin.install "nodi-darwin-#{arch}" => "nodi"
    chmod 0755, bin/"nodi"
    pkgshare.install "LICENSE"
  end

  test do
    assert_match "nodi #{version} darwin/", shell_output("#{bin}/nodi version")
    assert_match "nodi get", shell_output("#{bin}/nodi help")
  end
end
