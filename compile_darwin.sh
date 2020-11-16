#!/bin/sh

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

docker run -v "${DIR}:/build" golang:latest sh -c "cd /build && GOOS=darwin GOARCH=amd64 env go build"
