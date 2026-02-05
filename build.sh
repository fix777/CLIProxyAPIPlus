#!/bin/sh
set -e
set -u

CGO_ENABLED=0 go build -ldflags "-s -w \
  -X 'main.Version=6.7.45-1-plus' \
  -X 'main.Commit=$(git rev-parse --short HEAD)' \
  -X 'main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
  -o cli-proxy-api-plus ./cmd/server/
