class Nodi < Formula
  desc "Private engineering package transport via GitHub Releases"
  homepage "https://github.com/MaximSavhenko/nodi"
  version "0.3.0"
  license "MIT"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.3.0/nodi-darwin-arm64.zip"
      sha256 "d356de6ad10b4f4696ac73ceb2c0b83c898750ca1ddefa8a4405e7fb5be77721"
    end
    on_intel do
      url "https://github.com/MaximSavhenko/nodi/releases/download/v0.3.0/nodi-darwin-amd64.zip"
      sha256 "82de9f750a1bc3d392d72cf5f17800bbfba47c215875f960d8a8064def67c874"
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
