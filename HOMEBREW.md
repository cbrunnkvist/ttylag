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

## Maintenance

### Updating the Formula

When you release a new version (e.g., v0.1.2):

1. **Calculate new SHA256**:
   ```bash
   curl -sL https://github.com/cbrunnkvist/ttylag/archive/refs/tags/0.1.2.tar.gz | shasum -a 256
   ```

2. **Update the formula**:
   ```ruby
   url "https://github.com/cbrunnkvist/ttylag/archive/refs/tags/0.1.2.tar.gz"
   sha256 "NEW_SHA256_HERE"
   ```

3. **For Tap**: Push update to your tap repo
4. **For Core**: Open PR to homebrew-core updating the formula

### Automated Updates

Consider using [Homebrew Bump Formula](https://github.com/marketplace/actions/homebrew-bump-formula) GitHub Action to automatically create PRs when you release new versions.

## Key Learnings

1. **Homebrew 5.0 Changes**: 
   - Formulas must be in a tap (can't install from arbitrary paths)
   - `brew tap-new` creates the proper structure
   - Core submission process unchanged

2. **Go Formula Pattern**:
   - Use `depends_on "go" => :build`
   - Use `std_go_args(ldflags: "-s -w")` for consistency
   - Test with `--version` is standard practice

3. **Module Path Note**:
   - Your `go.mod` uses `github.com/user/ttylag`
   - Your repo is at `github.com/cbrunnkvist/ttylag`
   - This doesn't affect Homebrew (builds from tarball, not `go get`)
   - May want to update go.mod in future for consistency

## Files Created

- `Formula/ttylag.rb` - The Homebrew formula
- `.sisyphus/notepads/homebrew-process/` - Development notes

## Resources

- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae)
- [How to Create and Maintain a Tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
- [Homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core)
