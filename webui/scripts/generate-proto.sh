#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_ROOT="${SOURCE_ROOT:-$(cd "${ROOT}/../.." && pwd)}"
MAILBOX_PROTO_DIR="${MAILBOX_PROTO_DIR:-${SOURCE_ROOT}/mailbox/proto}"
COMMON_PROTO_DIR="${COMMON_PROTO_DIR:-${SOURCE_ROOT}/common-lib/proto}"
OUT_DIR="${OUT_DIR:-${ROOT}/src/proto}"
LOCAL_PLUGIN="${ROOT}/node_modules/.bin/protoc-gen-ts_proto"
AGGREGATE_PLUGIN="${SOURCE_ROOT}/webui/node_modules/.bin/protoc-gen-ts_proto"
PLUGIN="${PROTOC_GEN_TS_PROTO:-}"

if [[ -z "${PLUGIN}" ]]; then
  if [[ -x "${LOCAL_PLUGIN}" ]]; then
    PLUGIN="${LOCAL_PLUGIN}"
  elif [[ -x "${AGGREGATE_PLUGIN}" ]]; then
    PLUGIN="${AGGREGATE_PLUGIN}"
  fi
fi

if [[ -z "${PLUGIN}" || ! -x "${PLUGIN}" ]]; then
  printf 'ts-proto plugin not found; run npm install in webui first\n' >&2
  exit 1
fi
if [[ ! -f "${MAILBOX_PROTO_DIR}/email.proto" || ! -f "${MAILBOX_PROTO_DIR}/mailbox_service.proto" || ! -f "${COMMON_PROTO_DIR}/byte/v/forge/contracts/mailbox/v1/mailbox.proto" ]]; then
  printf 'mailbox proto not found under: %s\n' "${MAILBOX_PROTO_DIR}" >&2
  exit 1
fi

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

protoc -I "${MAILBOX_PROTO_DIR}" -I "${COMMON_PROTO_DIR}" \
  --plugin="protoc-gen-ts_proto=${PLUGIN}" \
  --ts_proto_out="${OUT_DIR}" \
  --ts_proto_opt=onlyTypes=true,outputServices=none,esModuleInterop=true,useJsonWireFormat=true,snakeToCamel=false \
  --ts_proto_opt=Mbyte/v/forge/contracts/mailbox/v1/mailbox.proto=@byte-v-forge/common-ui/proto/byte/v/forge/contracts/mailbox/v1/mailbox \
  "${MAILBOX_PROTO_DIR}/email.proto" \
  "${MAILBOX_PROTO_DIR}/mailbox_service.proto"
