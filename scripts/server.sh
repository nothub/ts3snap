#!/usr/bin/env nix-shell
#! nix-shell -I nixpkgs=https://github.com/NixOS/nixpkgs/archive/c3aa7b8938b17aebd2deecf7be0636000d62a2b9.tar.gz
#! nix-shell -p docker openssl
#! nix-shell -i sh --pure
# shellcheck shell=sh

set -eu
cd "$(dirname "$(realpath "$0")")/.."

if test ! -d run/ssl; then
    mkdir -p run/ssl
    openssl req \
        -x509 \
        -nodes \
        -newkey rsa:4096 \
        -out run/ssl/crt.pem \
        -keyout run/ssl/key.pem \
        -days 7 \
        -subj "/C=XX/ST=X/L=X/O=X/OU=X/CN=X"
fi

docker run -it --rm \
    -e TS3SERVER_LICENSE=accept \
    -p 127.0.0.1:9987:9987/udp \
    -p 127.0.0.1:30033:30033 \
    -p 127.0.0.1:10011:10011 \
    -p 127.0.0.1:10022:10022 \
    -p 127.0.0.1:10080:10080 \
    -p 127.0.0.1:10443:10443 \
    -v ${PWD}/run/ssl:/etc/ssl/query \
    "n0thub/ts3" \
    "query_protocols=raw,ssh,http,https" \
    "query_https_certificate_file=/etc/ssl/query/crt.pem" \
    "query_https_private_key_file=/etc/ssl/query/key.pem"
