.PHONY: fmt fmt-check lint test build check install-lint sign probe-apple-vm release-local

fmt:
	@gofmt -w $$(find . -name '*.go' -type f)

fmt-check:
	@out="$$(gofmt -l $$(find . -name '*.go' -type f))"; \
	if [ -n "$$out" ]; then \
		echo "The following files are not gofmt-formatted:"; \
		echo "$$out"; \
		exit 1; \
	fi

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Run 'make install-lint' first."; \
		exit 1; \
	}
	@golangci-lint run ./...

test:
	@go test ./...

build:
	@mkdir -p bin
	@go build -o bin/vibebox ./cmd/vibebox

sign: build
	@./scripts/sign-vibebox.sh ./bin/vibebox

probe-apple-vm: sign
	@./bin/vibebox probe --json --provider apple-vm

release-local:
	@./scripts/build-release.sh dev dist

check: fmt-check lint test build

install-lint:
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
