# Agent Instructions

This project is managed with the help of AI agents. To maintain consistency and quality:

1. **Follow the Style Guide**: Adhere to the formatting and structural conventions seen in the existing codebase.
2. **Update Documentation**: When adding features or changing behavior, update `README.md`, `DESIGN.md`, and `ttylag.1` (man page).
   - Use `make man` to regenerate the man page from `cmd/genman/main.go`.
   - CI will fail if `ttylag.1` does not match the output of `make man`.
3. **README Maintenance**:
   - The "Flags" section in `README.md` must match the verbatim output of `ttylag --help`.
   - The "Preset Profiles" section in `README.md` must match the verbatim output of `ttylag --list-profiles`.
   - If you modify CLI flags or profiles, you **MUST** update these sections in `README.md`.
4. **Test Thoroughly**:
   - Run `go test ./...` for unit tests.
   - Run `./smoke_test.sh` for integration/sanity checks.
   - Use the `verify_` tools in `cmd/` for quantitative verification of timing/bandwidth.
5. **Keep it Clean**: Ensure `go mod tidy` is run and code is formatted with `go fmt`.
6. **Homebrew Formula**: When releasing a new version, update the Homebrew formula:
   - Update `Formula/ttylag.rb` with the new version tag and SHA256
   - The formula uses the GitHub release tarball (not `go install`)
   - Calculate SHA256: `curl -sL https://github.com/cbrunnkvist/ttylag/archive/refs/tags/VERSION.tar.gz | shasum -a 256`
   - Update the tap repository (cbrunnkvist/homebrew-tap) with the new formula