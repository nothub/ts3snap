#!/usr/bin/env nix-shell
#! nix-shell -I nixpkgs=https://github.com/NixOS/nixpkgs/archive/c3aa7b8938b17aebd2deecf7be0636000d62a2b9.tar.gz
#! nix-shell -p go_1_22
#! nix-shell -i sh --pure
# shellcheck shell=sh

set -eu
cd "$(dirname "$(realpath "$0")")/.."

go test -v -vet='all' './...'
