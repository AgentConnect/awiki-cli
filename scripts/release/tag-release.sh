#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required to run scripts/release/tag-release.sh" >&2
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

TAG="v${VERSION}"

# Require clean working tree to避免把未提交修改发布出去
if [ -n "$(git status --porcelain)" ]; then
  echo "Error: working tree is not clean; please commit or stash changes before tagging" >&2
  exit 1
fi

# Ensure we are on a named branch, not detached HEAD
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "${BRANCH}" = "HEAD" ]; then
  echo "Error: currently on a detached HEAD; please checkout a branch (e.g. main) before tagging" >&2
  exit 1
fi

# Ensure the branch has an upstream and is fully pushed
if ! git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
  echo "Error: current branch ${BRANCH} has no upstream; please set upstream and push before tagging" >&2
  exit 1
fi

if [ -n "$(git cherry)" ]; then
  echo "Error: there are local commits not pushed to origin; please push them before tagging" >&2
  exit 1
fi

# Check for existing tag locally and remotely
if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  echo "Error: tag ${TAG} already exists locally" >&2
  exit 1
fi

if git ls-remote --tags origin "refs/tags/${TAG}" | grep -q .; then
  echo "Error: tag ${TAG} already exists on origin" >&2
  exit 1
fi

echo "Creating tag ${TAG} on branch ${BRANCH}..."
git tag -a "${TAG}" -m "Release ${TAG}"

echo "Pushing tag ${TAG} to origin..."
git push origin "${TAG}"

echo "Done. CI should pick up tag ${TAG} and run the release workflow."
cat <<EOF

Expected canonical release assets after the workflow finishes:
- dist/update-metadata.json
- dist/public-skill/awiki-cli/SKILL.md

The external deploy system must then publish:
- update metadata to the awiki update service
- the root skill file to https://awiki.ai/skills/awiki-cli/SKILL.md

EOF

cat <<EOF

After the tag-triggered workflow completes, verify that the canonical release assets were produced:
- dist/update-metadata.json
- dist/public-skill/awiki-cli/SKILL.md

The external deploy system must then publish:
- update metadata to the awiki update service
- the root skill file to https://awiki.ai/skills/awiki-cli/SKILL.md

EOF
