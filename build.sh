#!/usr/bin/env bash

set -e

APP=gtr
SRC=./cmd/gtr
OUT=./build
VERSION="0.0.1"

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

    DIR="$OUT/$VERSION-$GOOS-$GOARCH"
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
