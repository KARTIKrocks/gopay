GOLANGCI_LINT_VERSION := v2.12.2
GOIMPORTS_VERSION := v0.45.0

MODULES = . ./stripe ./paypal ./razorpay
SUB_MODULES = ./stripe ./paypal ./razorpay

.PHONY: all setup ci test test-race coverage lint lint-fix fix fmt fmt-check vet tidy build bench clean

all: tidy fmt vet lint build test

## Install development tools (skips if already present)
setup:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	}
	@command -v goimports >/dev/null 2>&1 || { \
		echo "Installing goimports $(GOIMPORTS_VERSION)..."; \
		go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION); \
	}

## CI: run lint and tests with race detector (used in CI pipelines)
ci: fmt-check vet lint test-race

## Build all modules
build:
	@for mod in $(MODULES); do \
		echo "==> Building $$mod"; \
		(cd $$mod && go build ./...) || exit 1; \
	done

## Run tests across all modules
test:
	@for mod in $(MODULES); do \
		echo "==> Testing $$mod"; \
		(cd $$mod && go test ./...) || exit 1; \
	done

## Run tests with race detector
test-race:
	@for mod in $(MODULES); do \
		echo "==> Testing (race) $$mod"; \
		(cd $$mod && go test -race -count=1 ./...) || exit 1; \
	done

## Run tests with coverage and generate report
coverage:
	@go test -race -coverprofile=coverage-core.out -covermode=atomic ./...
	@cd stripe && go test -race -coverprofile=../coverage-stripe.out -covermode=atomic ./...
	@cd paypal && go test -race -coverprofile=../coverage-paypal.out -covermode=atomic ./...
	@cd razorpay && go test -race -coverprofile=../coverage-razorpay.out -covermode=atomic ./...
	@cat coverage-core.out > coverage.out
	@tail -n +2 coverage-stripe.out >> coverage.out
	@tail -n +2 coverage-paypal.out >> coverage.out
	@tail -n +2 coverage-razorpay.out >> coverage.out
	@go tool cover -func=coverage.out | tail -1
	@echo "Full report: go tool cover -html=coverage.out"

## Run linter across all modules
lint: setup
	@for mod in $(MODULES); do \
		echo "==> Linting $$mod"; \
		(cd $$mod && golangci-lint run --timeout=5m ./...) || exit 1; \
	done

## Run golangci-lint with auto-fix
lint-fix: setup
	@for mod in $(MODULES); do \
		echo "==> Linting (fix) $$mod"; \
		(cd $$mod && golangci-lint run --fix ./...) || exit 1; \
	done

## Fix code formatting and linting issues
fix: fmt lint-fix

## Format code
fmt: setup
	@gofmt -s -w .
	@goimports -w .

## Check formatting without modifying files (used in CI)
fmt-check: setup
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || { echo "Unformatted files found. Run 'make fmt'."; exit 1; }
	@test -z "$$(goimports -l . | tee /dev/stderr)" || { echo "Unordered imports found. Run 'make fmt'."; exit 1; }

## Run go vet across all modules
vet:
	@for mod in $(MODULES); do \
		echo "==> Vetting $$mod"; \
		(cd $$mod && go vet ./...) || exit 1; \
	done

## Run go mod tidy across all modules
tidy:
	@for mod in $(MODULES); do \
		echo "==> Tidying $$mod"; \
		(cd $$mod && go mod tidy) || exit 1; \
	done

## Run benchmarks
bench:
	@for mod in $(MODULES); do \
		echo "==> Benchmarking $$mod"; \
		(cd $$mod && go test -bench=. -benchmem ./...) || exit 1; \
	done

## Remove coverage files
clean:
	@rm -f coverage*.out

