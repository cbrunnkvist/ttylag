.PHONY: build test release

BINARY_NAME=ttylag

build:
	go build -o $(BINARY_NAME) .

test:
	go test -v ./...
	./smoke_test.sh

release:
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG= env var not set (e.g. make release TAG=v0.1.0)"; \
		printf "Latest release: "; \
		git describe --tags --abbrev=0 2>/dev/null || echo "none"; \
		exit 1; \
	fi
	@if [[ ! "$(TAG)" =~ ^v ]]; then \
		echo "Error: TAG must start with 'v' (e.g. v0.1.0)"; \
		exit 1; \
	fi
	@if git rev-parse "$(TAG)" >/dev/null 2>&1; then \
		echo "Error: Tag $(TAG) already exists"; \
		exit 1; \
	fi
	git tag -a $(TAG) -m "Release $(TAG)"
	git push origin $(TAG)
