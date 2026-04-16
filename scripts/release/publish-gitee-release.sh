#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v curl >/dev/null 2>&1; then
  echo "Error: curl is required to run scripts/release/publish-gitee-release.sh" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required to run scripts/release/publish-gitee-release.sh" >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  echo "Error: git is required to run scripts/release/publish-gitee-release.sh" >&2
  exit 1
fi

usage() {
  cat <<'EOF'
Usage: scripts/release/publish-gitee-release.sh [tag]

Examples:
  scripts/release/publish-gitee-release.sh
  scripts/release/publish-gitee-release.sh v0.0.1-beta.16

Required environment variables:
  GITEE_TOKEN      Gitee personal access token with repo/release permissions

Optional environment variables:
  GITEE_USERNAME   Gitee login username for HTTPS git authentication
  GITEE_OWNER      Gitee repository owner (default: agentconnect)
  GITEE_REPO       Gitee repository name (default: awiki-cli)
  GITEE_GIT_URL    Optional git remote URL for pushing tags to Gitee
                   Example: git@gitee.com:agentconnect/awiki-cli.git
  GITEE_API_PROXY  Optional proxy URL for Gitee API requests. By default,
                   Gitee API requests are sent directly.
  GITEE_API_NO_PROXY Optional no_proxy value for Gitee API requests
  GITHUB_OWNER     GitHub repository owner (default: AgentConnect)
  GITHUB_REPO      GitHub repository name (default: awiki-cli)
  GITHUB_TOKEN     Optional GitHub token for higher API rate limits
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

VERSION=""
if [[ -f package.json ]]; then
  VERSION="$(jq -r '.version // empty' package.json)"
fi

TAG="${1:-}"
if [[ -z "${TAG}" && -n "${VERSION}" ]]; then
  TAG="v${VERSION}"
fi

if [[ -z "${TAG}" ]]; then
  echo "Error: tag argument is required when package.json.version is unavailable." >&2
  usage >&2
  exit 1
fi

GITEE_OWNER="${GITEE_OWNER:-agentconnect}"
GITEE_REPO="${GITEE_REPO:-awiki-cli}"
GITHUB_OWNER="${GITHUB_OWNER:-AgentConnect}"
GITHUB_REPO="${GITHUB_REPO:-awiki-cli}"
GITEE_USERNAME="${GITEE_USERNAME:-}"
GITEE_TOKEN="${GITEE_TOKEN:-}"
GITEE_GIT_URL="${GITEE_GIT_URL:-}"
GITEE_API_PROXY="${GITEE_API_PROXY:-}"
GITEE_API_NO_PROXY="${GITEE_API_NO_PROXY:-}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"

if [[ -z "${GITEE_TOKEN}" ]]; then
  echo "Error: GITEE_TOKEN is required." >&2
  exit 1
fi

if [[ -z "${GITEE_GIT_URL}" ]]; then
  if [[ -z "${GITEE_USERNAME}" ]]; then
    echo "Error: GITEE_USERNAME is required when GITEE_GIT_URL is not set." >&2
    exit 1
  fi
  GITEE_GIT_URL="https://${GITEE_USERNAME}:${GITEE_TOKEN}@gitee.com/${GITEE_OWNER}/${GITEE_REPO}.git"
fi

normalize_proxy_value() {
  local raw="${1:-}"

  # Some shells or wrapper scripts may export malformed values like
  # `https_proxy=http://127.0.0.1:7897`. Accept both the standard URL form
  # and this nested `name=value` form.
  case "${raw}" in
    http_proxy=*|https_proxy=*|HTTP_PROXY=*|HTTPS_PROXY=*|all_proxy=*|ALL_PROXY=*|no_proxy=*|NO_PROXY=*)
      raw="${raw#*=}"
      ;;
  esac

  raw="${raw%\"}"
  raw="${raw#\"}"
  raw="${raw%\'}"
  raw="${raw#\'}"

  printf '%s\n' "${raw}"
}

github_curl_proxy_url="$(normalize_proxy_value "${HTTPS_PROXY:-${https_proxy:-${ALL_PROXY:-${all_proxy:-${HTTP_PROXY:-${http_proxy:-}}}}}}")"
github_curl_no_proxy="$(normalize_proxy_value "${NO_PROXY:-${no_proxy:-}}")"
gitee_curl_proxy_url="$(normalize_proxy_value "${GITEE_API_PROXY}")"
gitee_curl_no_proxy="$(normalize_proxy_value "${GITEE_API_NO_PROXY}")"

curl_github() {
  if [[ -n "${github_curl_proxy_url}" ]]; then
    if [[ -n "${github_curl_no_proxy}" ]]; then
      curl --proxy "${github_curl_proxy_url}" --noproxy "${github_curl_no_proxy}" "$@"
      return
    fi
    curl --proxy "${github_curl_proxy_url}" "$@"
    return
  fi

  if [[ -n "${github_curl_no_proxy}" ]]; then
    curl --noproxy "${github_curl_no_proxy}" "$@"
    return
  fi

  curl "$@"
}

curl_gitee() {
  if [[ -n "${gitee_curl_proxy_url}" ]]; then
    if [[ -n "${gitee_curl_no_proxy}" ]]; then
      curl --proxy "${gitee_curl_proxy_url}" --noproxy "${gitee_curl_no_proxy}" "$@"
      return
    fi
    curl --proxy "${gitee_curl_proxy_url}" "$@"
    return
  fi

  if [[ -n "${gitee_curl_no_proxy}" ]]; then
    curl --noproxy "${gitee_curl_no_proxy}" "$@"
    return
  fi

  curl "$@"
}

github_api_headers=(
  -H "Accept: application/vnd.github+json"
  -H "X-GitHub-Api-Version: 2022-11-28"
)
if [[ -n "${GITHUB_TOKEN}" ]]; then
  github_api_headers+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/awiki-gitee-release.XXXXXX")"
release_json="${tmp_dir}/github-release.json"
gitee_release_json="${tmp_dir}/gitee-release.json"
create_json="${tmp_dir}/gitee-create.json"
upload_json="${tmp_dir}/gitee-upload.json"
download_dir="${tmp_dir}/downloads"
mkdir -p "${download_dir}"

cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

echo "Fetching GitHub release metadata for ${TAG}..."
if [[ -n "${github_curl_proxy_url}" ]]; then
  echo "Using curl proxy for GitHub API requests and release downloads."
fi
github_status="$(curl_github -sS -L -o "${release_json}" -w '%{http_code}' \
  "${github_api_headers[@]}" \
  "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/tags/${TAG}")"

if [[ "${github_status}" != "200" ]]; then
  echo "Error: failed to fetch GitHub release metadata for ${TAG} (HTTP ${github_status})." >&2
  cat "${release_json}" >&2
  exit 1
fi

release_id="$(jq -r '.id // empty' "${release_json}")"
if [[ -z "${release_id}" ]]; then
  echo "Error: GitHub release metadata did not include an id." >&2
  cat "${release_json}" >&2
  exit 1
fi

release_name="$(jq -r '.name // empty' "${release_json}")"
if [[ -z "${release_name}" ]]; then
  release_name="${TAG}"
fi
release_body="$(jq -r '.body // ""' "${release_json}")"
release_target="$(jq -r '.target_commitish // empty' "${release_json}")"
prerelease_flag="$(jq -r 'if .prerelease then "true" else "false" end' "${release_json}")"

asset_count="$(jq '.assets | length' "${release_json}")"
if [[ "${asset_count}" -eq 0 ]]; then
  echo "Error: GitHub release ${TAG} has no uploaded assets to mirror." >&2
  exit 1
fi

asset_manifest="${tmp_dir}/asset-manifest.tsv"
jq -r '.assets[] | [.name, .browser_download_url] | @tsv' "${release_json}" > "${asset_manifest}"

echo "Ensuring local tag ${TAG} exists..."
if ! git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  git fetch origin "refs/tags/${TAG}:refs/tags/${TAG}"
fi

echo "Pushing tag ${TAG} to Gitee..."
git remote add gitee "${GITEE_GIT_URL}" 2>/dev/null || \
  git remote set-url gitee "${GITEE_GIT_URL}"
git push gitee "refs/tags/${TAG}:refs/tags/${TAG}"

assets_dir="${download_dir}"
echo "Downloading GitHub release assets to ${assets_dir}..."
while IFS=$'\t' read -r asset_name asset_url; do
  if [[ -z "${asset_name}" || -z "${asset_url}" ]]; then
    continue
  fi

  echo "Downloading ${asset_name}..."
  curl_github --fail --location --progress-bar \
    --output "${assets_dir}/${asset_name}" \
    "${asset_url}"
done < "${asset_manifest}"

echo "Looking up Gitee release for ${TAG}..."
gitee_lookup_status="$(curl_gitee -sS -L -o "${gitee_release_json}" -w '%{http_code}' \
  "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases/tags/${TAG}?access_token=${GITEE_TOKEN}")"

gitee_release_id=""
if [[ "${gitee_lookup_status}" == "200" ]]; then
  gitee_release_id="$(jq -r 'if type == "object" then (.id // empty) else empty end' "${gitee_release_json}")"
fi

if [[ -n "${gitee_release_id}" ]]; then
  echo "Reusing existing Gitee release for ${TAG}."
else
  echo "Creating Gitee release for ${TAG}..."
  create_args=(
    -X POST
    -H "Content-Type: application/x-www-form-urlencoded"
    --data-urlencode "access_token=${GITEE_TOKEN}"
    --data-urlencode "tag_name=${TAG}"
    --data-urlencode "name=${release_name}"
    --data-urlencode "body=${release_body}"
    --data-urlencode "prerelease=${prerelease_flag}"
  )

  if [[ -n "${release_target}" ]]; then
    create_args+=(--data-urlencode "target_commitish=${release_target}")
  fi

  create_status="$(curl_gitee -sS -L -o "${create_json}" -w '%{http_code}' \
    "${create_args[@]}" \
    "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases")"

  gitee_release_id="$(jq -r 'if type == "object" then (.id // empty) else empty end' "${create_json}")"
  if [[ -n "${gitee_release_id}" ]]; then
    cp "${create_json}" "${gitee_release_json}"
    echo "Created Gitee release for ${TAG}."
  else
    echo "Create response did not include an id; re-querying Gitee by tag..."
    gitee_lookup_status="$(curl_gitee -sS -L -o "${gitee_release_json}" -w '%{http_code}' \
      "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases/tags/${TAG}?access_token=${GITEE_TOKEN}")"
    if [[ "${gitee_lookup_status}" == "200" ]]; then
      gitee_release_id="$(jq -r 'if type == "object" then (.id // empty) else empty end' "${gitee_release_json}")"
    fi
  fi

  if [[ -z "${gitee_release_id}" ]]; then
    echo "Error: failed to create or look up Gitee release for ${TAG}." >&2
    echo "Create response:" >&2
    cat "${create_json}" >&2
    echo >&2
    echo "Lookup response:" >&2
    cat "${gitee_release_json}" >&2
    exit 1
  fi

  if [[ "${create_status}" != "201" && "${create_status}" != "200" ]]; then
    echo "Warning: Gitee create release returned HTTP ${create_status}, but the release is now queryable." >&2
  fi
fi

existing_assets="$(jq -r '.assets[]?.name' "${gitee_release_json}")"

local_assets=()
while IFS= read -r asset_path; do
  local_assets+=("${asset_path}")
done < <(find "${assets_dir}" -maxdepth 1 -type f | sort)

if [[ "${#local_assets[@]}" -eq 0 ]]; then
  echo "Error: no local assets are available in ${assets_dir}." >&2
  exit 1
fi

for asset_path in "${local_assets[@]}"; do
  asset_name="$(basename "${asset_path}")"
  if printf '%s\n' "${existing_assets}" | grep -Fxq "${asset_name}"; then
    echo "Skipping existing Gitee asset ${asset_name}."
    continue
  fi

  asset_size="$(wc -c < "${asset_path}" | tr -d '[:space:]')"
  echo "Uploading ${asset_name} to Gitee (${asset_size} bytes)..."
  upload_status="$(curl_gitee --fail-with-body --location --progress-bar \
    --output "${upload_json}" \
    --write-out '%{http_code}' \
    --connect-timeout 15 \
    -X POST \
    -F "access_token=${GITEE_TOKEN}" \
    -F "owner=${GITEE_OWNER}" \
    -F "repo=${GITEE_REPO}" \
    -F "release_id=${gitee_release_id}" \
    -F "file=@${asset_path}" \
    "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases/${gitee_release_id}/attach_files")"

  if [[ "${upload_status}" != "201" && "${upload_status}" != "200" ]]; then
    echo "Error: failed to upload ${asset_name} to Gitee (HTTP ${upload_status})." >&2
    cat "${upload_json}" >&2
    exit 1
  fi

  echo "Uploaded ${asset_name} to Gitee."
done

echo
echo "Done."
echo "GitHub Release: https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/tag/${TAG}"
echo "Gitee Release: https://gitee.com/${GITEE_OWNER}/${GITEE_REPO}/releases/tag/${TAG}"
