.PHONY: build test vet verify

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

verify: build test vet
