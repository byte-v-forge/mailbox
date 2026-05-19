#!/usr/bin/env bash
set -Eeuo pipefail

pids=()

cleanup() {
  local status=$?
  if ((${#pids[@]} > 0)); then
    kill "${pids[@]}" >/dev/null 2>&1 || true
    wait "${pids[@]}" >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

mailbox_pg_dsn=${MAILBOX_PG_DSN:-}
if [[ -z "$mailbox_pg_dsn" ]]; then
  echo "MAILBOX_PG_DSN is required" >&2
  exit 1
fi

outlook_addr=${MAILBOX_OUTLOOK_INTERNAL_ADDR:-127.0.0.1:50052}
webhook_addr=${MAILBOX_WEBHOOK_HTTP_ADDR:-:8082}
frpc_config_file=${MAILBOX_FRPC_CONFIG_FILE:-/etc/frp/frpc.toml}
frpc_enable=${MAILBOX_FRPC_ENABLE:-false}

if [[ -n "${MAILBOX_FRPC_CONFIG:-}" ]]; then
  mkdir -p "$(dirname "$frpc_config_file")"
  printf '%s\n' "$MAILBOX_FRPC_CONFIG" > "$frpc_config_file"
  unset MAILBOX_FRPC_CONFIG
  frpc_enable=true
fi

(
  export LISTEN_ADDR="$outlook_addr"
  export PG_DSN="$mailbox_pg_dsn"
  export WEBHOOK_HTTP_ADDR="$webhook_addr"
  export FRP_ENABLE="$frpc_enable"
  export FRP_CONFIG_FILE="$frpc_config_file"
  exec /app/bin/outlook-mailbox
) &
pids+=("$!")

export LISTEN_ADDR=${MAILBOX_LISTEN_ADDR:-:50051}
export MAILBOX_PG_DSN="$mailbox_pg_dsn"
export MAILBOX_EMAIL_PROVIDER_ADDR="$outlook_addr"
exec /app/bin/mailbox
