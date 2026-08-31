.PHONY: test vet lint gosec govulncheck ci

test:
	go test ./... -v

vet:
	go vet ./...

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found; installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	golangci-lint run ./...

gosec:
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "gosec not found; installing..."; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	gosec ./...

govulncheck:
	@if ! command -v govulncheck >/dev/null 2>&1; then \
		echo "govulncheck not found; installing..."; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
	fi
	govulncheck ./...

ci: test vet lint gosec govulncheck
