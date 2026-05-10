#!/usr/bin/env sh
set -eu
make tidy
make build
make test
make lint
