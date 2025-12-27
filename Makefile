# Simple developer convenience Makefile

.PHONY: build test fmt docs

build:
	go build ./cmd/backend

test:
	go test ./...

fmt:
	go fmt ./...

docs:
	@echo "Docs are in the docs/ directory"
	@ls -1 docs | sed -e 's/^/ - /'