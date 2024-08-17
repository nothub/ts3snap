#!/usr/bin/env sh

set -eu
cd "$(dirname "$(realpath "$0")")/.."

>&2 echo "building snapshot binaries"
goreleaser build --clean --snapshot
