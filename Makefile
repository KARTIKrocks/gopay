GOLANGCI_LINT_VERSION := v2.13.2
GOIMPORTS_VERSION := v0.50.0
GOVULNCHECK_VERSION := v1.8.0

# The Markdown linter. Versioned in website/package.json rather than pinned
# here, so Dependabot keeps it current along with the rest of the docs toolchain.
MARKDOWNLINT := website/node_modules/.bin/markdownlint-cli2

MODULES = . ./stripe ./paypal ./razorpay
SUB_MODULES = ./stripe ./paypal ./razorpay

.PHONY: all setup ci test test-race coverage lint lint-fix fix fmt fmt-check vet tidy build bench clean vuln lint-docs lint-docs-fix print-golangci-lint-version print-govulncheck-version

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

## CI: run what the required `ci` gate job runs for Go changes (fmt, vet,
## lint, race tests, vuln scan). Docs changes should also run `make
## lint-docs` — left out here since it needs `npm ci` in website/ first and
## most local iteration doesn't touch docs.
ci: fmt-check vet lint test-race vuln

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

## Scan every module for known vulnerabilities, filtered to advisories this
## code actually reaches. Mirrors the `vuln` CI job, which gates merges.
##
## Needs network access — the advisory database is fetched on every run.
##
## Note this also scans the standard library of whichever Go toolchain you have
## installed, so it can fail locally on a green branch when your Go is a patch
## release behind the one CI pins. That is a real finding about your machine,
## not a false positive.
vuln:
	@command -v govulncheck >/dev/null 2>&1 || { \
		echo "Installing govulncheck $(GOVULNCHECK_VERSION)..."; \
		go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION); \
	}
	@for mod in $(MODULES); do \
		echo "==> Scanning $$mod"; \
		(cd $$mod && govulncheck ./...) || exit 1; \
	done

## Lint every Markdown file in the repo — the docs site, the README, and the
## contributor/security policies. Config and rationale live in
## .markdownlint-cli2.jsonc. Needs Node; the binary comes from website/, which
## is the only npm project here.
lint-docs:
	@test -x "$(MARKDOWNLINT)" || { echo "Run 'npm ci' in website/ first."; exit 1; }
	@$(MARKDOWNLINT)

## Auto-fix what markdownlint-cli2 can fix automatically.
lint-docs-fix:
	@test -x "$(MARKDOWNLINT)" || { echo "Run 'npm ci' in website/ first."; exit 1; }
	@$(MARKDOWNLINT) --fix

## Print the pinned scanner version. CI installs govulncheck with this rather
## than hardcoding a second copy of the number, so the workflow and this file
## cannot drift apart.
print-govulncheck-version:
	@echo $(GOVULNCHECK_VERSION)

## Print the pinned linter version. CI resolves golangci-lint-action's version
## input from this rather than hardcoding a second copy of the number, so the
## workflow and this file cannot drift apart.
print-golangci-lint-version:
	@echo $(GOLANGCI_LINT_VERSION)

## Remove coverage files
clean:
	@rm -f coverage*.out

