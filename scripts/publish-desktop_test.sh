#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/publish-desktop.sh
source "${script_dir}/publish-desktop.sh"

assert_equal() {
  local want="$1"
  local got="$2"
  [[ "${got}" == "${want}" ]] || {
    printf 'expected %s, got %s\n' "${want}" "${got}" >&2
    exit 1
  }
}

assert_equal v0.16.2 "$(next_version v0.16.1 patch)"
assert_equal v0.17.0 "$(next_version v0.16.1 minor)"
assert_equal v1.0.0 "$(next_version v0.16.1 major)"
version_is_greater v0.16.2 v0.16.1
version_is_greater v0.17.0 v0.16.99
version_is_greater v1.0.0 v0.99.99
if version_is_greater v0.16.1 v0.16.1; then
  printf 'equal versions must not compare as newer\n' >&2
  exit 1
fi

printf 'PACT release version tests passed.\n'
