#!/bin/sh
set -e
set -u

CGO_ENABLED=0 go build -ldflags "-s -w \
  -X 'main.Version=v6.8.28-1-3-plus' \
  -X 'main.Commit=$(git rev-parse --short HEAD)' \
  -X 'main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
  -o cli-proxy-api-plus_v6.8.28-1-3-plus ./cmd/server/
