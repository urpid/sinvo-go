#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"
mkdir -p builds

GOOS=linux GOARCH=amd64 go build -ldflags "-X main.devMode=false" -o builds/sinvo-go-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -ldflags "-X main.devMode=false" -o builds/sinvo-go-linux-arm64 .
GOOS=windows GOARCH=amd64 go build -ldflags "-X main.devMode=false" -o builds/sinvo-go-windows-amd64.exe .
