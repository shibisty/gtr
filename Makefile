# Make recipes must be indented with a tab.
.PHONY: build test vet build-win build-linux build-macos build-macos-arm

build:
	go build -o build/gtr ./cmd/gtr
test:
	go test -race ./...
vet:
	go vet ./...
build-win:
	GOOS=windows GOARCH=amd64 go build -o build/gtr.exe ./cmd/gtr
build-linux:
	GOOS=linux GOARCH=amd64 go build -o build/gtr-linux ./cmd/gtr
build-macos:
	GOOS=darwin GOARCH=amd64 go build -o build/gtr-darwin ./cmd/gtr
build-macos-arm:
	GOOS=darwin GOARCH=arm64 go build -o build/gtr-darwin-arm64 ./cmd/gtr
