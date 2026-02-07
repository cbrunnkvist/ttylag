.PHONY: build test release check-clean check-man

BINARY_NAME=ttylag

build:
	go build -o $(BINARY_NAME) .

man:
	go run cmd/genman/main.go > ttylag.1

check-man:
	go run cmd/genman/main.go > ttylag.1.tmp
	diff -u ttylag.1 ttylag.1.tmp || (echo "Error: ttylag.1 is out of date. Run 'make man' and commit the changes." && rm ttylag.1.tmp && exit 1)
	@rm ttylag.1.tmp

check-clean:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory is not clean. Commit or stash changes first."; \
		git status; \
		exit 1; \
	fi

test:
	go test -v ./...
	./smoke_test.sh

release: check-clean check-man test
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG not set. Usage: make release TAG=0.1.3"; \
		printf "Latest release: "; \
		git describe --tags --abbrev=0 2>/dev/null || echo "none"; \
		exit 1; \
	fi
	@if [[ ! "$(TAG)" =~ ^[0-9]+\. ]]; then \
		echo "Error: TAG must start with a digit (e.g. 0.1.3)"; \
		exit 1; \
	fi
	@if git rev-parse "$(TAG)" >/dev/null 2>&1; then \
		echo "Error: Tag $(TAG) already exists"; \
		exit 1; \
	fi
	@echo "Releasing version $(TAG)..."
	@sed -i '' 's/var version = "[^"]*"/var version = "$(TAG)"/' main.go
	@git add main.go
	@git commit -m "chore: bump version to $(TAG)"
	@git tag -a $(TAG) -m "Release $(TAG)"
	@git push origin main
	@git push origin $(TAG)
	@echo "✓ Released $(TAG)"
	@echo "  GitHub Actions will build binaries and update Homebrew tap"

brew-sha256:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make brew-sha256 VERSION=0.1.2"; \
		echo "Calculate SHA256 for a GitHub release tarball"; \
		exit 1; \
	fi
	@echo "SHA256 for v$(VERSION):"
	@curl -sL https://github.com/cbrunnkvist/ttylag/archive/refs/tags/$(VERSION).tar.gz | shasum -a 256 | cut -d' ' -f1
