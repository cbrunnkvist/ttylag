.PHONY: build test release

BINARY_NAME=ttylag

build:
	go build -o $(BINARY_NAME) .

test:
	go test -v ./...
	./smoke_test.sh

release:
	@if [ -z "$(VERSION)" ]; then 
		echo "Error: VERSION= env var not set (e.g. make release VERSION=v0.1.0)"; 
		printf "Latest release: "; 
		git describe --tags --abbrev=0 2>/dev/null || echo "none"; 
		exit 1; 
	fi
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then 
		echo "Error: Tag $(VERSION) already exists"; 
		exit 1; 
	fi
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)
