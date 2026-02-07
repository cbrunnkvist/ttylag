# Homebrew Distribution Guide for ttylag

This document explains how ttylag is distributed via Homebrew and the process for maintaining it.

## Quick Summary

✅ **Status**: ttylag is now available via a custom Homebrew tap  
✅ **Install Command**: `brew tap cbrunnkvist/tap && brew install ttylag`  
🔄 **Next Goal**: Submit to Homebrew Core for `brew install ttylag`

## What Was Done

### 1. Created Homebrew Formula

**File**: `Formula/ttylag.rb`

```ruby
class Ttylag < Formula
  desc "Userspace PTY wrapper that simulates laggy/slow network connections"
  homepage "https://github.com/cbrunnkvist/ttylag"
  url "https://github.com/cbrunnkvist/ttylag/archive/refs/tags/0.1.1.tar.gz"
  sha256 "5cf3012601ca611dd3bafc9279e329234d75a6be152d81e261e02f0db13b4d16"
  license "MIT"

  depends_on "go" => :build

  def install
    ldflags = "-s -w"
    system "go", "build", *std_go_args(ldflags: ldflags)
  end

  test do
    system "#{bin}/ttylag", "--version"
  end
end
```

### 2. Created Homebrew Tap

**Command used**:
```bash
brew tap-new cbrunnkvist/tap
```

This created a tap at: `/opt/homebrew/Library/Taps/cbrunnkvist/homebrew-tap/`

The tap contains:
- Formula directory for your formulas
- GitHub Actions workflows for testing and publishing
- README with tap documentation

### 3. Tested Installation

✅ Formula builds successfully  
✅ Binary installs correctly  
✅ Tests pass (`brew test ttylag`)  
✅ Works on macOS (arm64 tested)

## Current State: Tap Distribution

Users can now install ttylag via:

```bash
brew tap cbrunnkvist/tap
brew install ttylag
```

Or in one line:
```bash
brew tap cbrunnkvist/tap && brew install ttylag
```

### To Publish Your Tap to GitHub

1. **Create a GitHub repository** named `homebrew-tap`
   - This creates the URL: `github.com/cbrunnkvist/homebrew-tap`

2. **Push the tap**:
   ```bash
   cd /opt/homebrew/Library/Taps/cbrunnkvist/homebrew-tap
   git remote add origin git@github.com:cbrunnkvist/homebrew-tap.git
   git add Formula/ttylag.rb
   git commit -m "Add ttylag formula v0.1.1"
   git push -u origin main
   ```

3. **Users can then install**:
   ```bash
   brew tap cbrunnkvist/tap
   brew install ttylag
   ```

## Future Goal: Homebrew Core

For `brew install ttylag` (without tap), you need to submit to Homebrew Core.

### Requirements for Homebrew Core

From [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae):

1. **Stable version** ✅ (v0.1.1 tagged)
2. **Open source** ✅ (MIT license)
3. **Notability**: Homebrew requires projects to be "notable" enough:
   - At least 30 forks
   - At least 30 watchers
   - OR established in the community
4. **Builds from source** ✅ (formula uses source tarball)
5. **Has a test** ✅ (version check test included)

### Submission Process to Homebrew Core

1. **Fork homebrew-core**:
   ```bash
   git clone https://github.com/Homebrew/homebrew-core.git
   cd homebrew-core
   ```

2. **Add your formula**:
   ```bash
   cp /path/to/your/Formula/ttylag.rb Formula/t/ttylag.rb
   ```
   Note: Homebrew uses alphabetical subdirectories (t/ for ttylag)

3. **Audit the formula**:
   ```bash
   brew audit --new --formula ttylag
   ```

4. **Test locally**:
   ```bash
   brew install --build-from-source ttylag
   brew test ttylag
   ```

5. **Commit and push**:
   ```bash
   git add Formula/t/ttylag.rb
   git commit -m "ttylag 0.1.1 (new formula)"
   git push origin main
   ```

6. **Open Pull Request**:
   - Go to https://github.com/Homebrew/homebrew-core/pulls
   - Create PR from your fork
   - Title: `ttylag 0.1.1 (new formula)`
   - Follow the PR template

7. **Wait for review**:
   - Homebrew maintainers will review
   - CI will run tests on multiple macOS versions and Linux
   - May request changes

8. **Merge**:
   - Once approved, it will be merged
   - Bottles (pre-built binaries) will be generated automatically

## Automated Formula Updates (Recommended)

The easiest way to update the Homebrew formula is to **let GoReleaser do it automatically**.

### How It Works

GoReleaser has built-in Homebrew tap support. When you push a new tag:

1. GoReleaser builds all binaries
2. Creates the GitHub release
3. **Automatically generates and pushes the updated formula** to your tap
4. No manual SHA256 calculation needed!

### Setup

1. **Configure .goreleaser.yaml** (already done):
   ```yaml
   brews:
     - name: ttylag
       repository:
         owner: cbrunnkvist
         name: homebrew-tap
         token: "{{ .Env.HOMEBREW_TOKEN }}"
       directory: Formula
       homepage: "https://github.com/cbrunnkvist/ttylag"
       description: "Userspace PTY wrapper that simulates laggy/slow network connections"
       license: "MIT"
       test: |
         system "#{bin}/ttylag", "--version"
       commit_msg_template: "Brew formula update for {{ .ProjectName }} version {{ .Tag }}"
   ```

2. **Add HOMEBREW_TOKEN secret** to your GitHub repository:
   - Go to Settings → Secrets and variables → Actions
   - Click "New repository secret"
   - Name: `HOMEBREW_TOKEN`
   - Value: Create a Personal Access Token with `repo` scope
   - The token needs write access to `cbrunnkvist/homebrew-tap`

3. **Release workflow** (already configured):
   The `.github/workflows/release.yml` passes the token to GoReleaser.

### Creating a Release (Automated)

Simply push a new tag:

```bash
make release TAG=0.1.3
```

Or manually:
```bash
git tag 0.1.3
git push origin 0.1.3
```

GoReleaser will automatically:
- Build all binaries
- Create GitHub release
- **Update the Homebrew formula** with correct version and SHA256
- Push to `cbrunnkvist/homebrew-tap`

## Manual Formula Updates (If Needed)

If you need to manually update the formula (e.g., hotfix without new release):

### Calculate SHA256

Use the Makefile helper:
```bash
make brew-sha256 VERSION=0.1.3
# Output: SHA256 for v0.1.3:
#         33fe94fcbdd6429eff2bd34ba141a2791581afc8e92c6c75258743498841d9ac
```

Or manually:
```bash
curl -sL https://github.com/cbrunnkvist/ttylag/archive/refs/tags/0.1.3.tar.gz | shasum -a 256
```

### Update Formula

Edit `Formula/ttylag.rb`:
```ruby
url "https://github.com/cbrunnkvist/ttylag/archive/refs/tags/0.1.3.tar.gz"
sha256 "33fe94fcbdd6429eff2bd34ba141a2791581afc8e92c6c75258743498841d9ac"
```

Then commit and push to your tap repository.

## Key Learnings

1. **Homebrew 5.0 Changes**: 
   - Formulas must be in a tap (can't install from arbitrary paths)
   - `brew tap-new` creates the proper structure
   - Core submission process unchanged

2. **Go Formula Pattern**:
   - Use `depends_on "go" => :build`
   - Use `std_go_args(ldflags: "-s -w")` for consistency
   - Test with `--version` is standard practice

## Files Created

- `Formula/ttylag.rb` - The Homebrew formula
- `.sisyphus/notepads/homebrew-process/` - Development notes

## Resources

- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae)
- [How to Create and Maintain a Tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
- [Homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core)
