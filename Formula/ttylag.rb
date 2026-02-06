class Ttylag < Formula
  desc "Userspace PTY wrapper that simulates laggy/slow network connections"
  homepage "https://github.com/cbrunnkvist/ttylag"
  url "https://github.com/cbrunnkvist/ttylag/archive/refs/tags/0.1.1.tar.gz"
  sha256 "5cf3012601ca611dd3bafc9279e329234d75a6be152d81e261e02f0db13b4d16"
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
