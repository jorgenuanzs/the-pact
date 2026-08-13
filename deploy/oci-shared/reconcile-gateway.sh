#!/usr/bin/env bash
set -Eeuo pipefail

snippet_file="/opt/the-pact/shared/pact.Caddyfile"
magi_caddyfile="$(readlink -f /opt/nuanzs-magi/current/Caddyfile)"
edge_network="nuanzs-edge"
begin_marker="# BEGIN THE-PACT MANAGED ROUTE"
end_marker="# END THE-PACT MANAGED ROUTE"

[[ -f "${snippet_file}" ]] || {
  echo "Missing PACT Caddy snippet: ${snippet_file}" >&2
  exit 1
}
[[ -f "${magi_caddyfile}" ]] || {
  echo "Missing active Magi Caddyfile." >&2
  exit 1
}

caddy_container="$(
  docker ps \
    --filter label=com.docker.compose.project=nuanzs-infra \
    --filter label=com.docker.compose.service=caddy \
    --format '{{.ID}}' \
    | head -n 1
)"
[[ -n "${caddy_container}" ]] || {
  echo "The Magi Caddy container is not running." >&2
  exit 1
}

changed=false
docker network inspect "${edge_network}" >/dev/null 2>&1 || docker network create "${edge_network}" >/dev/null
if ! docker inspect "${caddy_container}" --format '{{json .NetworkSettings.Networks}}' | grep -q "\"${edge_network}\""; then
  docker network connect "${edge_network}" "${caddy_container}"
  changed=true
fi

temporary_file="$(mktemp)"
cleanup() {
  rm -f -- "${temporary_file}"
}
trap cleanup EXIT

awk -v begin="${begin_marker}" -v end="${end_marker}" '
  $0 == begin { managed = 1; next }
  $0 == end { managed = 0; next }
  !managed {
    count++
    lines[count] = $0
    if ($0 != "") {
      last_nonempty = count
    }
  }
  END {
    for (line_number = 1; line_number <= last_nonempty; line_number++) {
      print lines[line_number]
    }
  }
' "${magi_caddyfile}" >"${temporary_file}"

{
  printf '\n%s\n' "${begin_marker}"
  cat "${snippet_file}"
  printf '%s\n' "${end_marker}"
} >>"${temporary_file}"

if ! cmp -s "${temporary_file}" "${magi_caddyfile}"; then
  # Keep the inode because Docker bind-mounts this exact file into Caddy.
  # Replacing it atomically would leave the running container on the old inode.
  cat "${temporary_file}" >"${magi_caddyfile}"
  chmod 640 "${magi_caddyfile}"
  changed=true
fi

docker exec "${caddy_container}" caddy validate --config /etc/caddy/Caddyfile >/dev/null
if [[ "${changed}" == true ]]; then
  docker exec "${caddy_container}" caddy reload --config /etc/caddy/Caddyfile >/dev/null
fi
