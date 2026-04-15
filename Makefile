MISE ?= mise
GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt fmt-check lint test check

fmt:
	@if [ -n "$(GOFILES)" ]; then $(MISE) exec -- gofmt -w $(GOFILES); fi

fmt-check:
	@if [ -n "$(GOFILES)" ] && [ -n "$$($(MISE) exec -- gofmt -l $(GOFILES))" ]; then \
		echo "Run 'make fmt' to format Go files."; \
		$(MISE) exec -- gofmt -l $(GOFILES); \
		exit 1; \
	fi

lint:
	$(MISE) exec -- golangci-lint run ./...

test:
	$(MISE) exec -- go test ./...

check: fmt-check test lint
