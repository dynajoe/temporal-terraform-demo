.PHONY: lint build test

default: build

lint:
	go vet ./...

build:
	go build -v ./...

test:
	go test ./...
