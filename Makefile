.PHONY: fmt vet lint test check hooks install-tools

# Format code (gofmt always; goimports if installed).
fmt:
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . || true

vet:
	go vet ./...

# Run golangci-lint if installed, otherwise hint how to get it.
lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... \
		|| echo "golangci-lint not installed — run 'make install-tools'"

test:
	go test ./...

# Everything the pre-push hook runs.
check: fmt vet lint test

# Enable the versioned git hooks (.githooks/pre-push).
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks enabled: .githooks"

# Install the optional dev tools.
install-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
