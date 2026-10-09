#!/usr/bin/env bash
# Builds release archives in the format gtr installs from (ADR-0008):
#
#   dist/gtr_<version>_<os>_<arch>.zip      (Windows)
#   dist/gtr_<version>_<os>_<arch>.tar.gz   (other systems)
#   dist/SHA256SUMS
#
# Usage: ./release.sh 0.2.0
# The release workflow runs it on a v* tag and uploads dist/ to the GitHub release.
set -euo pipefail

APP=gtr
SRC=./cmd/gtr
VERSION=${1:?usage: ./release.sh <version>}
VERSION=${VERSION#v}
OUT=./dist

rm -rf "$OUT"
mkdir -p "$OUT"

platforms=(
    "windows amd64"
    "windows arm64"
    "linux amd64"
    "linux arm64"
    "darwin amd64"
    "darwin arm64"
)

for platform in "${platforms[@]}"; do
    read -r GOOS GOARCH <<<"$platform"
    name="${APP}_${VERSION}_${GOOS}_${GOARCH}"
    work="$OUT/$name"
    mkdir -p "$work"
    bin="$APP"
    [ "$GOOS" = windows ] && bin="$APP.exe"

    echo "Building $name"
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -trimpath \
        -ldflags="-s -w -X main.Version=$VERSION" \
        -o "$work/$bin" \
        "$SRC"
    cp README.md "$work/" 2>/dev/null || true

    if [ "$GOOS" = windows ]; then
        (cd "$work" && zip -q -X "../$name.zip" ./*)
    else
        tar -C "$work" -czf "$OUT/$name.tar.gz" .
    fi
    rm -rf "$work"
done

(cd "$OUT" && sha256sum ./*.zip ./*.tar.gz | sed 's# \./# #' > SHA256SUMS)
echo
cat "$OUT/SHA256SUMS"
