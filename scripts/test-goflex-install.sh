#!/usr/bin/env sh
set -eu
GOBIN="${GOBIN:-$(go env GOPATH)/bin}"
go install ./cmd/goflex
"$GOBIN/goflex" version >/dev/null
