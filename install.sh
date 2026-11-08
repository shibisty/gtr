#!/bin/sh
# Installs gtr on Linux and macOS:
#
#   curl -fsSL https://raw.githubusercontent.com/shibisty/gtr/main/install.sh | sh
#
# What it does:
#   1. downloads the gtr release archive for this system and SHA256SUMS from
#      GitHub releases and checks the archive's SHA-256;
#   2. puts gtr into ~/.gtr/bin (GTR_HOME/bin) and creates the go and gofmt
#      shims there (`gtr self shims`);
#   3. adds ~/.gtr/bin to PATH in your shell profile.
#
# Settings (environment variables):
#   GTR_VERSION=0.2.0       a version instead of the latest release
#   GTR_HOME=/opt/gtr       the gtr home instead of ~/.gtr
#   GTR_NO_SHIMS=1          do not create the go/gofmt shims
#   GTR_NO_MODIFY_PATH=1    do not touch shell profiles
#   GITHUB_TOKEN=...        for the GitHub API rate limit
set -eu

REPO=shibisty/gtr
API=${GTR_GITHUB_API:-https://api.github.com}
DOWNLOAD=${GTR_DOWNLOAD_BASE:-https://github.com/$REPO/releases/download}
GTR_HOME=${GTR_HOME:-$HOME/.gtr}
BIN="$GTR_HOME/bin"

say() { printf '%s\n' "gtr-install: $*"; }
die() { printf '%s\n' "gtr-install: error: $*" >&2; exit 1; }

fetch() { # fetch <url> <file>
    if command -v curl >/dev/null 2>&1; then
        if [ -n "${GITHUB_TOKEN:-}" ] && [ "${1#"$API"}" != "$1" ]; then
            curl -fsSL --retry 3 -H "Authorization: Bearer $GITHUB_TOKEN" -o "$2" "$1"
        else
            curl -fsSL --retry 3 -o "$2" "$1"
        fi
    elif command -v wget >/dev/null 2>&1; then
        if [ -n "${GITHUB_TOKEN:-}" ] && [ "${1#"$API"}" != "$1" ]; then
            wget -q --tries=3 --header="Authorization: Bearer $GITHUB_TOKEN" -O "$2" "$1"
        else
            wget -q --tries=3 -O "$2" "$1"
        fi
    else
        die "curl or wget is required"
    fi
}

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | cut -d' ' -f1
    else
        die "sha256sum or shasum is required to verify the download"
    fi
}

case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    MINGW* | MSYS* | CYGWIN*) die "on Windows use install.ps1: irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex" ;;
    *) die "unsupported system $(uname -s)" ;;
esac
case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "unsupported architecture $(uname -m)" ;;
esac

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t gtr)
trap 'rm -rf "$tmp"' EXIT INT TERM

version=${GTR_VERSION:-}
version=${version#v}
if [ -z "$version" ]; then
    fetch "$API/repos/$REPO/releases/latest" "$tmp/latest.json" || die "cannot query the latest release (set GTR_VERSION or GITHUB_TOKEN)"
    version=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' "$tmp/latest.json" | head -n 1)
    [ -n "$version" ] || die "no release found in $REPO"
fi

asset="gtr_${version}_${os}_${arch}.tar.gz"
say "downloading gtr $version for $os/$arch"
fetch "$DOWNLOAD/v$version/$asset" "$tmp/$asset" || die "cannot download $DOWNLOAD/v$version/$asset"
fetch "$DOWNLOAD/v$version/SHA256SUMS" "$tmp/SHA256SUMS" || die "cannot download SHA256SUMS of $version"
want=$(awk -v a="$asset" '{ n = $2; sub(/^\*/, "", n); if (n == a) print $1 }' "$tmp/SHA256SUMS" | tr 'A-F' 'a-f')
[ -n "$want" ] || die "SHA256SUMS does not list $asset"
got=$(sha256 "$tmp/$asset")
[ "$got" = "$want" ] || die "SHA-256 mismatch for $asset (expected $want, got $got)"

mkdir -p "$tmp/x" "$BIN"
tar -xzf "$tmp/$asset" -C "$tmp/x"
[ -f "$tmp/x/gtr" ] || die "$asset does not contain gtr"
chmod 755 "$tmp/x/gtr"
mv -f "$tmp/x/gtr" "$BIN/gtr.new"
mv -f "$BIN/gtr.new" "$BIN/gtr"
say "installed $BIN/gtr"

export GTR_HOME
if [ -z "${GTR_NO_SHIMS:-}" ]; then
    "$BIN/gtr" self shims >/dev/null || die "gtr self shims failed"
    say "created the go and gofmt shims in $BIN"
fi

line="export PATH=\"$BIN:\$PATH\""
[ "$GTR_HOME" = "$HOME/.gtr" ] || line="export GTR_HOME=\"$GTR_HOME\"; $line"
case ":$PATH:" in
    *":$BIN:"*) on_path=1 ;;
    *) on_path= ;;
esac
if [ -z "${GTR_NO_MODIFY_PATH:-}" ]; then
    # ~/.profile for login shells (sh, bash without ~/.bash_profile); the files
    # of the shells in use; zsh, the default on macOS, never reads ~/.profile.
    case "${SHELL:-}" in */zsh) zsh_user=1 ;; *) zsh_user= ;; esac
    changed=
    for rc in "$HOME/.profile" "$HOME/.bash_profile" "$HOME/.bashrc" "$HOME/.zshrc"; do
        case "$rc" in
            */.profile) ;;
            */.zshrc) [ -f "$rc" ] || [ -n "$zsh_user" ] || [ "$os" = darwin ] || continue ;;
            *) [ -f "$rc" ] || continue ;;
        esac
        if [ -f "$rc" ] && grep -F "$BIN" "$rc" >/dev/null 2>&1; then
            continue
        fi
        printf '\n# gtr\n%s\n' "$line" >>"$rc"
        changed="$changed $rc"
    done
    [ -z "$changed" ] || say "added $BIN to PATH in$changed"
fi
if [ -z "$on_path" ]; then
    say "open a new terminal, or run: $line"
fi
say "done: run \`gtr help\` to start"
