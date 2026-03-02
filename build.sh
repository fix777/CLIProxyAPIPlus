#!/bin/sh
set -e
set -u

VERSION="v6.8.37-0-0-plus"

CGO_ENABLED=0 go build -ldflags "-s -w \
  -X 'main.Version=${VERSION}' \
  -X 'main.Commit=$(git rev-parse --short HEAD)' \
  -X 'main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
  -o cli-proxy-api-plus_${VERSION} ./cmd/server/
