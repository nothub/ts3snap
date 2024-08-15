#!/usr/bin/env nix-shell
#! nix-shell -I nixpkgs=https://github.com/NixOS/nixpkgs/archive/c3aa7b8938b17aebd2deecf7be0636000d62a2b9.tar.gz
#! nix-shell -p go_1_22 upx
#! nix-shell -i sh --pure
# shellcheck shell=sh

set -eu
cd "$(dirname "$(realpath "$0")")/.."

build() (

    file="out/ts3snap_${1}_${2}"
    if test "$1" = "windows"; then
        file="${file}.exe"
    fi

    # build static binary
    GOOS="$1" GOARCH="$2" go build \
        -ldflags "-s -w" \
        -o "${file}" \
        .

    # compress with upx
    # ( except for mac because upx mac support requires a feature flag )
    if test "$1" != "darwin"; then
        upx --best --lzma \
            --no-color \
            --no-progress \
            --no-time \
            "${file}"
    fi

)

go clean
rm    -rf "out"
mkdir -p  "out"

build linux amd64
build linux arm64
build darwin amd64
build darwin arm64
build windows amd64
