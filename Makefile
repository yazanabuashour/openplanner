MISE ?= mise
GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')
GOVULNCHECK_VERSION ?= v1.2.0

.PHONY: fmt fmt-check lint test vuln-check validate-agent-skill check

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

vuln-check:
	$(MISE) exec -- go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

validate-agent-skill:
	./scripts/validate-agent-skill.sh skills/openplanner

check: fmt-check validate-agent-skill test vuln-check lint
