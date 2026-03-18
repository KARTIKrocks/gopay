MODULES = . ./stripe ./paypal ./razorpay
SUB_MODULES = ./stripe ./paypal ./razorpay
MODULE_PATH = github.com/KARTIKrocks/gopay

.PHONY: all ci test test-race coverage lint fmt vet tidy build bench clean release-prep release-local

all: tidy fmt lint test

## CI: run lint and tests with race detector (used in CI pipelines)
ci: fmt vet lint test-race

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
lint:
	@for mod in $(MODULES); do \
		echo "==> Linting $$mod"; \
		(cd $$mod && golangci-lint run --timeout=5m ./...) || exit 1; \
	done

## Format code
fmt:
	@gofmt -s -w .
	@goimports -w .

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

## Remove build artifacts and coverage files
clean:
	@rm -f coverage*.out
	@go clean -cache -testcache

## Prepare sub-modules for release: strip replace directives, set version
## Usage: make release-prep VERSION=v0.1.0
release-prep:
ifndef VERSION
	$(error VERSION is required. Usage: make release-prep VERSION=v0.1.0)
endif
	@for mod in $(SUB_MODULES); do \
		echo "==> release-prep $$mod"; \
		(cd $$mod && \
		go mod edit -dropreplace $(MODULE_PATH) && \
		go mod edit -require $(MODULE_PATH)@$(VERSION)) || exit 1; \
	done
	@echo ""
	@echo "Done! Sub-modules now point to $(MODULE_PATH)@$(VERSION)"
	@echo "Next steps:"
	@echo "  git add -A && git commit -m 'Prepare release $(VERSION)'"
	@echo "  git tag $(VERSION)"
	@echo "  git tag stripe/$(VERSION)"
	@echo "  git tag paypal/$(VERSION)"
	@echo "  git tag razorpay/$(VERSION)"
	@echo "  git push origin main --tags"

## Restore replace directives for local development after a release
release-local:
	@for mod in $(SUB_MODULES); do \
		echo "==> release-local $$mod"; \
		(cd $$mod && \
		go mod edit -replace $(MODULE_PATH)=../ && \
		go mod tidy) || exit 1; \
	done
	@echo ""
	@echo "Done! Sub-modules restored to local replace directives."
