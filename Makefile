export PATH := $(PATH):$(HOME)/sdk/go/bin:$(HOME)/go/bin

.PHONY: generate build test run
generate:
	templ generate
build: generate
	go build -o bin/monogatari .
test: generate
	go test ./...
run: build
	./bin/monogatari
