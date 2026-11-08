# Make recipes must be indented with a tab.
# Builds go through gtr-manager itself (the "build" block of gtr.json); with gtr
# installed, `gtr build`, `gtr build --all-targets --archive` and `gtr test` do the same.
# VERSION: the v* tag on HEAD, else the version in gtr.json (make release VERSION=1.2.3).
VERSION ?= $(shell git describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null | sed 's/^v//')
VFLAG := $(if $(VERSION),--version $(VERSION))
GM := build/.bootstrap/gtr-manager
.PHONY: build all release test vet bootstrap

bootstrap:
	go -C gtr-manager build -o ../$(GM) ./cmd/gtr-manager
build: bootstrap
	$(GM) build $(VFLAG)
all: bootstrap
	$(GM) build --all-targets $(VFLAG)
release: bootstrap
	$(GM) build --all-targets --archive $(VFLAG)
test:
	go test -race ./...
vet:
	go vet ./...
