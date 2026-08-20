#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
classifier="${script_dir}/classify-release.sh"
temporary_dir="$(mktemp -d)"

cleanup() {
  rm -rf -- "${temporary_dir}"
}
trap cleanup EXIT

current="${temporary_dir}/current"
candidate="${temporary_dir}/candidate"
changed="${temporary_dir}/changed.txt"
mkdir -p \
  "${current}/internal/transport/httpapi/adminui" \
  "${candidate}/internal/transport/httpapi/adminui" \
  "${current}/web/src" \
  "${candidate}/web/src" \
  "${current}/internal/server" \
  "${candidate}/internal/server"
printf 'same-handler\n' >"${current}/internal/transport/httpapi/adminui/adminui.go"
printf 'same-handler\n' >"${candidate}/internal/transport/httpapi/adminui/adminui.go"
printf 'old-ui\n' >"${current}/web/src/App.tsx"
printf 'new-ui\n' >"${candidate}/web/src/App.tsx"
printf 'same-backend\n' >"${current}/internal/server/server.go"
printf 'same-backend\n' >"${candidate}/internal/server/server.go"

profile="$(bash "${classifier}" "${current}" "${candidate}" "${changed}")"
[[ "${profile}" == "frontend" ]]
[[ "$(cat "${changed}")" == "web/src/App.tsx" ]]

printf 'old-ui\n' >"${candidate}/web/src/App.tsx"
printf 'changed-handler\n' >"${candidate}/internal/transport/httpapi/adminui/adminui.go"
profile="$(bash "${classifier}" "${current}" "${candidate}" "${changed}")"
[[ "${profile}" == "frontend" ]]
[[ "$(cat "${changed}")" == "internal/transport/httpapi/adminui/adminui.go" ]]

printf 'changed-backend\n' >"${candidate}/internal/server/server.go"
profile="$(bash "${classifier}" "${current}" "${candidate}" "${changed}")"
[[ "${profile}" == "full" ]]
grep -qx 'internal/server/server.go' "${changed}"

printf 'same-backend\n' >"${candidate}/internal/server/server.go"
printf 'same-handler\n' >"${candidate}/internal/transport/httpapi/adminui/adminui.go"
profile="$(bash "${classifier}" "${current}" "${candidate}" "${changed}")"
[[ "${profile}" == "full" ]]
[[ ! -s "${changed}" ]]

echo "Release classifier tests passed."
