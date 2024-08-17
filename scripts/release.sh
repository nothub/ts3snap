#!/usr/bin/env sh

set -eu
cd "$(dirname "$(realpath "$0")")/.."

>&2 echo "publishing full release"
goreleaser release --clean
