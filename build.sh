#!/usr/bin/env bash
# Cross-compiles gtr for every platform into build/<version>/<os>-<arch>/.
# Usage: ./build.sh [version]   (release archives: ./release.sh)

set -e

APP=gtr
SRC=./cmd/gtr
OUT=./build
# Version: the first argument, else the nearest git tag (v1.2.3 → 1.2.3, with
# -N-g<sha> after it on later commits), else "dev".
VERSION=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
VERSION=${VERSION#v}

echo "Cleaning..."
rm -rf "$OUT"
mkdir -p "$OUT"

platforms=(
    "windows amd64 .exe"
    "windows arm64 .exe"
    "linux amd64"
    "linux arm64"
    "darwin amd64"
    "darwin arm64"
)

for platform in "${platforms[@]}"; do

    read -r GOOS GOARCH EXT <<<"$platform"

    DIR="$OUT/$VERSION/$GOOS-$GOARCH"
    FILE="$DIR/$APP$EXT"

    mkdir -p "$DIR"

    echo "Building $FILE"

    CGO_ENABLED=0 \
    GOOS=$GOOS \
    GOARCH=$GOARCH \
    go build \
        -trimpath \
        -ldflags="-s -w -X main.Version=$VERSION" \
        -o "$FILE" \
        "$SRC"

done

echo
echo "Done."
