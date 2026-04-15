#!/usr/bin/env bash

release_root_dir() {
  cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
}

release_require_command() {
  local cmd="$1"
  local script_name="$2"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "Error: ${cmd} is required to run ${script_name}" >&2
    exit 1
  fi
}

release_read_version() {
  local root_dir="$1"
  if [[ ! -f "${root_dir}/package.json" ]]; then
    echo "Error: package.json not found in ${root_dir}" >&2
    exit 1
  fi

  local version
  version="$(jq -r '.version // empty' "${root_dir}/package.json")"
  if [[ -z "${version}" ]]; then
    echo "Error: .version is missing or empty in package.json" >&2
    exit 1
  fi

  printf '%s\n' "${version}"
}

release_require_clean_worktree() {
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "Error: working tree is not clean; please commit or stash changes before tagging" >&2
    exit 1
  fi
}

release_require_branch_with_upstream() {
  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  if [[ "${branch}" == "HEAD" ]]; then
    echo "Error: currently on a detached HEAD; please checkout a branch before tagging" >&2
    exit 1
  fi

  if ! git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
    echo "Error: current branch ${branch} has no upstream; please set upstream and push before tagging" >&2
    exit 1
  fi

  if [[ -n "$(git cherry)" ]]; then
    echo "Error: there are local commits not pushed to origin; please push them before tagging" >&2
    exit 1
  fi

  printf '%s\n' "${branch}"
}

release_require_tag_absent() {
  local tag="$1"

  if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
    echo "Error: tag ${tag} already exists locally" >&2
    exit 1
  fi

  if git ls-remote --tags origin "refs/tags/${tag}" | grep -q .; then
    echo "Error: tag ${tag} already exists on origin" >&2
    exit 1
  fi
}

release_create_and_push_tag() {
  local tag="$1"
  local message="$2"

  git tag -a "${tag}" -m "${message}"
  git push origin "${tag}"
}
