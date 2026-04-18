#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

check_coverage=0
show_profile_path=0
passthrough_args=()

while (($# > 0)); do
  case "$1" in
    --check)
      check_coverage=1
      ;;
    --show-profile)
      show_profile_path=1
      ;;
    *)
      passthrough_args+=("$1")
      ;;
  esac
  shift
done

cleanup_dir=""
coverprofile="${AWIKI_CLI_COVERPROFILE:-}"
if [[ -z "${coverprofile}" ]]; then
  cleanup_dir="$(mktemp -d "${TMPDIR:-/tmp}/awiki-cli-cover.XXXXXX")"
  coverprofile="${cleanup_dir}/unit-cover.out"
fi
mkdir -p "$(dirname "${coverprofile}")"

cleanup() {
  if [[ -n "${cleanup_dir}" ]]; then
    rm -rf "${cleanup_dir}"
  fi
}
trap cleanup EXIT

go_test_cmd=(go test ./... -coverprofile="${coverprofile}")
if ((${#passthrough_args[@]} > 0)); then
  go_test_cmd+=("${passthrough_args[@]}")
fi
"${go_test_cmd[@]}"

go tool cover -func="${coverprofile}" | tail -n 20

if [[ "${check_coverage}" == "1" ]]; then
  python3 scripts/check_go_coverage.py "${coverprofile}"
fi

if [[ "${show_profile_path}" == "1" || -n "${AWIKI_CLI_COVERPROFILE:-}" ]]; then
  echo "Coverage profile: ${coverprofile}"
fi
