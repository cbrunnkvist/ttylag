# Agent Instructions

This project is managed with the help of AI agents. To maintain consistency and quality:

1. **Follow the Style Guide**: Adhere to the formatting and structural conventions seen in the existing codebase.
2. **Update Documentation**: When adding features or changing behavior, update `README.md`, `DESIGN.md`, and `ttylag.1` (man page).
3. **README Maintenance**:
   - The "Flags" section in `README.md` must match the verbatim output of `ttylag --help`.
   - The "Preset Profiles" section in `README.md` must match the verbatim output of `ttylag --list-profiles`.
   - If you modify CLI flags or profiles, you **MUST** update these sections in `README.md`.
4. **Test Thoroughly**:
   - Run `go test ./...` for unit tests.
   - Run `./smoke_test.sh` for integration/sanity checks.
   - Use the `verify_` tools in `cmd/` for quantitative verification of timing/bandwidth.
5. **Keep it Clean**: Ensure `go mod tidy` is run and code is formatted with `go fmt`.