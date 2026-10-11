class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.1.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.1.0/nodi-darwin-arm64.zip"
      sha256 "4e9816d4debb6cbf2e6bd44bb1f27260d0a331b060fd6085ebf99b6ad14116d3"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.1.0/nodi-darwin-amd64.zip"
      sha256 "3970e957915eb4803cde554b8e54e8cd7926079d8723990e843c00ada9fb7c91"
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
