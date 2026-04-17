#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v tar >/dev/null 2>&1; then
  echo "Error: tar is required to render the public skill package" >&2
  exit 1
fi

VERSION="$(node -p "require('./package.json').version")"
ARCHIVE_PATH="dist/awiki-cli-${VERSION}-linux-amd64.tar.gz"
if [ ! -f "${ARCHIVE_PATH}" ]; then
  echo "Error: expected release archive ${ARCHIVE_PATH} not found" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

tar -xzf "${ARCHIVE_PATH}" -C "${TMP_DIR}"
BINARY_PATH="$(find "${TMP_DIR}" -type f -name awiki-cli | head -n 1)"
if [ -z "${BINARY_PATH}" ]; then
  echo "Error: awiki-cli binary not found in ${ARCHIVE_PATH}" >&2
  exit 1
fi
chmod +x "${BINARY_PATH}"

OUTPUT_DIR="dist/public-skill"
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

"${BINARY_PATH}" skill export --dir "${OUTPUT_DIR}" --all --include-root --clean

echo "Rendered public skill package into ${OUTPUT_DIR}" 
