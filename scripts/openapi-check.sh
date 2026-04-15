#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
TARGET_DIR="${ROOT_DIR}/sdk/generated"

cleanup() {
  rm -rf "${TMP_DIR}"
}

trap cleanup EXIT

"${ROOT_DIR}/scripts/openapi-generate.sh" "${TMP_DIR}"

if ! diff -ru "${TMP_DIR}" "${TARGET_DIR}"; then
  echo "Generated Go client is out of date. Run 'make openapi-generate'." >&2
  exit 1
fi
