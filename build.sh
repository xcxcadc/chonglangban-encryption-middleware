#!/usr/bin/env sh
set -eu

mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/chonglangban-middleware-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o dist/chonglangban-middleware-linux-arm64 .
cp .env.example dist/.env.example
echo "Built dist/chonglangban-middleware-linux-amd64 and dist/chonglangban-middleware-linux-arm64"
