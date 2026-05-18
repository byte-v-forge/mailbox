#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CONTRACTS_ROOT="$ROOT/../contracts"
PATH="$(go env GOPATH)/bin:$PATH"

rm -rf "$ROOT/gen/go"
mkdir -p "$ROOT/gen/go"

protoc -I "$ROOT/proto" -I "$ROOT/../contracts/mailbox/proto" \
  --go_out="$ROOT/gen/go" \
  --go_opt=paths=source_relative \
  --go-grpc_out="$ROOT/gen/go" \
  --go-grpc_opt=paths=source_relative \
  "$ROOT/proto/byte/v/forge/mailbox/internal/v1/mailbox_internal.proto" \
  "$ROOT/proto/byte/v/forge/mailbox/providers/imap/v1/imap.proto" \
  "$ROOT/proto/byte/v/forge/mailbox/providers/jmap/v1/jmap.proto"
