# Make recipes must be indented with a tab.
VERSION ?= $(or $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//'),dev)
LDFLAGS := -X main.Version=$(VERSION)
.PHONY: build test vet build-win build-linux build-macos build-macos-arm

build:
	go build -ldflags "$(LDFLAGS)" -o build/gtr ./cmd/gtr
test:
	go test -race ./...
vet:
	go vet ./...
build-win:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/gtr.exe ./cmd/gtr
build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/gtr-linux ./cmd/gtr
build-macos:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/gtr-darwin ./cmd/gtr
build-macos-arm:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/gtr-darwin-arm64 ./cmd/gtr
