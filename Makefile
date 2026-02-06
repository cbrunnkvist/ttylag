.PHONY: build test release

BINARY_NAME=ttylag

build:
	go build -o $(BINARY_NAME) .

man:
	go run cmd/genman/main.go > ttylag.1

check-man:
	go run cmd/genman/main.go > ttylag.1.tmp
	diff -u ttylag.1 ttylag.1.tmp || (echo "Error: ttylag.1 is out of date. Run 'make man' and commit the changes." && rm ttylag.1.tmp && exit 1)
	@rm ttylag.1.tmp

test:
	go test -v ./...
	./smoke_test.sh

release:
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG= env var not set (e.g. make release TAG=0.1.0)"; \
		printf "Latest release: "; \
		git describe --tags --abbrev=0 2>/dev/null || echo "none"; \
		exit 1; \
	fi
	@if [[ ! "$(TAG)" =~ ^[0-9]+\. ]]; then \
		echo "Error: TAG must start with a digit followed by a period (e.g. 0.1.0)"; \
		exit 1; \
	fi
	@if git rev-parse "$(TAG)" >/dev/null 2>&1; then \
		echo "Error: Tag $(TAG) already exists"; \
		exit 1; \
	fi
	git tag -a $(TAG) -m "Release $(TAG)"
	git push origin $(TAG)

brew-sha256:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make brew-sha256 VERSION=0.1.2"; \
		echo "Calculate SHA256 for a GitHub release tarball"; \
		exit 1; \
	fi
	@echo "SHA256 for v$(VERSION):"
	@curl -sL https://github.com/cbrunnkvist/ttylag/archive/refs/tags/$(VERSION).tar.gz | shasum -a 256 | cut -d' ' -f1
