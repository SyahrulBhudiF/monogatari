export PATH := $(PATH):$(HOME)/sdk/go/bin:$(HOME)/go/bin

.PHONY: generate build test run tools fmt lint check
generate:
	templ generate
build: generate
	go build -o bin/monogatari .
test: generate
	go test -race ./...
run: build
	./bin/monogatari
tools:
	scripts/install-tools.sh
fmt:
	golangci-lint fmt
lint:
	golangci-lint fmt --diff
	golangci-lint run
# check is what CI runs: generated files and go.mod are current, formatting and lint pass, tests pass.
check: generate
	git diff --exit-code -- '*_templ.go'
	go mod tidy -diff
	$(MAKE) lint
	go test -race ./...
	shellcheck --severity=warning .agents/setup .agents/resume scripts/*.sh
	actionlint
