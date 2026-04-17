#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

VERSION="$(node -p "require('./package.json').version")"
METADATA_FILE="dist/update-metadata.json"
ROOT_SKILL_FILE="dist/public-skill/awiki-cli/SKILL.md"
CHECKSUM_FILE="dist/awiki-cli-${VERSION}-checksums.txt"

for required in "${METADATA_FILE}" "${ROOT_SKILL_FILE}" "${CHECKSUM_FILE}"; do
  if [ ! -f "${required}" ]; then
    echo "Error: required release asset missing: ${required}" >&2
    exit 1
  fi
done

echo "Verified release assets exist:" 
for required in "${METADATA_FILE}" "${ROOT_SKILL_FILE}" "${CHECKSUM_FILE}"; do
  echo "  - ${required}"
done
