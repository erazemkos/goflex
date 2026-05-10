.PHONY: test lint build tidy install e2e codegen

test:
	go test ./... -race -cover

lint:
	golangci-lint run

build:
	go build ./...

tidy:
	go mod tidy

install:
	go install ./cmd/goflex

e2e:
	go test ./examples/todo/... -tags=e2e

codegen:
	./scripts/test-codegen.sh
