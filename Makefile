.PHONY: build test lint clean run

BINARY := pwman
MAIN := ./cmd/pwman

build:
	go build -o $(BINARY) $(MAIN)

run:
	go run $(MAIN)

test:
	go test -v ./...

test-coverage:
	go test -cover ./...

lint:
	golangci-lint run

fmt:
	go fmt ./...

clean:
	rm -f $(BINARY)

deps:
	go mod download
	go mod tidy

release-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 $(MAIN)

release-darwin:
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 $(MAIN)
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 $(MAIN)

release: release-linux release-darwin
