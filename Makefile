MISE ?= mise
GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt fmt-check lint test openapi-lint openapi-generate openapi-check check

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

openapi-lint:
	docker run --rm \
		-v "$(CURDIR):/work" \
		stoplight/spectral:6.15.0 lint /work/openapi/openapi.yaml --ruleset /work/.spectral.yaml

openapi-generate:
	./scripts/openapi-generate.sh

openapi-check:
	./scripts/openapi-check.sh

check: fmt-check openapi-lint openapi-check test lint
