#!/usr/bin/env sh

set -eu
cd "$(dirname "$(realpath "$0")")/.."

>&2 echo "running vet and tests"
go test -v -vet='all' './...'
