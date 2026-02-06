class Ttylag < Formula
  desc "Userspace PTY wrapper that simulates laggy/slow network connections"
  homepage "https://github.com/cbrunnkvist/ttylag"
  url "https://github.com/cbrunnkvist/ttylag/archive/refs/tags/0.1.2.tar.gz"
  sha256 "33fe94fcbdd6429eff2bd34ba141a2791581afc8e92c6c75258743498841d9ac"
  license "MIT"

  depends_on "go" => :build

  def install
    # Standard Go build with minimal symbols and stripped debug info
    ldflags = "-s -w"
    system "go", "build", *std_go_args(ldflags: ldflags)
  end

  test do
    # Basic sanity check to ensure the binary runs
    system "#{bin}/ttylag", "--version"
  end
end
