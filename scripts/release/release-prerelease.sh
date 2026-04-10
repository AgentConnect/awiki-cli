#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

DIST_TAG="${1:-}"

if [ -z "${DIST_TAG}" ]; then
  echo "Usage: scripts/release/release-prerelease.sh <dist-tag>" >&2
  echo "Example: scripts/release/release-prerelease.sh beta" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required to run scripts/release/release-prerelease.sh" >&2
  exit 1
fi

if [ ! -f package.json ]; then
  echo "Error: package.json not found in ${ROOT_DIR}" >&2
  exit 1
fi

VERSION="$(jq -r '.version // empty' package.json)"
if [ -z "${VERSION}" ]; then
  echo "Error: .version is missing or empty in package.json" >&2
  exit 1
fi

if [[ "${VERSION}" != *-* ]]; then
  echo "Error: pre-release version must contain a '-' suffix (e.g. 0.2.0-beta.1), got ${VERSION}" >&2
  exit 1
fi

TAG="v${VERSION}"

if [ -n "$(git status --porcelain)" ]; then
  echo "Error: working tree is not clean; please commit or stash changes before tagging" >&2
  exit 1
fi

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "${BRANCH}" = "HEAD" ]; then
  echo "Error: currently on a detached HEAD; please checkout a branch before tagging" >&2
  exit 1
fi

if ! git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
  echo "Error: current branch ${BRANCH} has no upstream; please set upstream and push before tagging" >&2
  exit 1
fi

if [ -n "$(git cherry)" ]; then
  echo "Error: there are local commits not pushed to origin; please push them before tagging" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  echo "Error: tag ${TAG} already exists locally" >&2
  exit 1
fi

if git ls-remote --tags origin "refs/tags/${TAG}" | grep -q .; then
  echo "Error: tag ${TAG} already exists on origin" >&2
  exit 1
fi

echo "Creating pre-release tag ${TAG} (dist-tag: ${DIST_TAG}) on branch ${BRANCH}..."
git tag -a "${TAG}" -m "Pre-release ${TAG} (dist-tag: ${DIST_TAG})"

echo "Pushing tag ${TAG} to origin..."
git push origin "${TAG}"

cat <<EOF

Pre-release tag ${TAG} has been pushed.

Next steps:
- CI will build binaries and create a GitHub pre-release for ${TAG}.
- To publish the npm pre-release package with dist-tag "${DIST_TAG}", run:

    NODE_AUTH_TOKEN=... npm publish --access public --tag ${DIST_TAG}

EOF

