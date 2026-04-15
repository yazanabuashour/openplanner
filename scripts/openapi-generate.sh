#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GENERATOR_IMAGE="${OPENAPI_GENERATOR_IMAGE:-openapitools/openapi-generator-cli:v7.20.0}"
OUTPUT_DIR="${1:-${ROOT_DIR}/sdk/generated}"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "${TMP_DIR}"
}

trap cleanup EXIT

docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volume "${ROOT_DIR}:/local" \
  "${GENERATOR_IMAGE}" generate \
  -g go \
  -i /local/openapi/openapi.yaml \
  -o /local/.openapi-generator-tmp \
  --package-name generated \
  --additional-properties=withGoMod=false,isGoSubmodule=true,generateInterfaces=true,hideGenerationTimestamp=true,enumClassPrefix=true \
  --global-property=apiDocs=false,modelDocs=false,apiTests=false,modelTests=false >/dev/null

mkdir -p "${OUTPUT_DIR}"
find "${OUTPUT_DIR}" -type f -name '*.go' -delete
find "${ROOT_DIR}/.openapi-generator-tmp" -maxdepth 1 -type f -name '*.go' -exec cp {} "${OUTPUT_DIR}/" \;
rm -rf "${ROOT_DIR}/.openapi-generator-tmp"

gofmt -w "${OUTPUT_DIR}"/*.go
