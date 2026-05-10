#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/shared"
cat > "$TMP/go.mod" <<EOF
module example.com/codegentest

go 1.22

require github.com/erazemkos/goflex v0.0.0

replace github.com/erazemkos/goflex => $ROOT
EOF
cat > "$TMP/shared/endpoints.go" <<'EOF'
package shared

import "github.com/erazemkos/goflex/pkg/api"

type Todo struct {
	ID    uint   `json:"id"`
	Title string `json:"title"`
}

type CreateTodoRequest struct {
	Title string `json:"title"`
}

var CreateTodo = api.Endpoint[CreateTodoRequest, Todo]{
	Method:      "POST",
	Path:        "/todos",
	Description: "creates a todo",
}
EOF
BIN="$TMP/goflex"
(cd "$ROOT" && go build -o "$BIN" ./cmd/goflex)
(
	cd "$TMP"
	"$BIN" generate --only api
	go mod tidy
	go build ./...
	SUM1=$(shasum generated/gen_server.go generated/gen_client.go)
	"$BIN" generate --only api | grep 'no changes'
	SUM2=$(shasum generated/gen_server.go generated/gen_client.go)
	test "$SUM1" = "$SUM2"
)
