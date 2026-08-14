#!/usr/bin/env bash
set -Eeuo pipefail

incoming_dir="${1:-}"
runtime_root="/opt/the-pact"
shared_dir="${runtime_root}/shared"
releases_dir="${runtime_root}/releases"
lock_file="${runtime_root}/deploy.lock"
builder_name="the-pact-builder-prod"
integration_suffix=""
integration_database_container=""
integration_network=""
integration_image=""

cleanup_integration() {
  if [[ -n "${integration_database_container}" ]]; then
    docker rm --force "${integration_database_container}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${integration_network}" ]]; then
    docker network rm "${integration_network}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${integration_image}" ]]; then
    docker image rm "${integration_image}" >/dev/null 2>&1 || true
  fi
}

trap cleanup_integration EXIT

if [[ -z "${incoming_dir}" || ! -d "${incoming_dir}" ]]; then
  echo "Usage: activate-release.sh /tmp/the-pact-<release>" >&2
  exit 2
fi

for command_name in docker flock openssl sha256sum tar; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "Missing required command: ${command_name}" >&2
    exit 1
  }
done

mkdir -p "${runtime_root}" "${shared_dir}" "${releases_dir}"
chmod 750 "${runtime_root}" "${shared_dir}" "${releases_dir}"
exec 9>"${lock_file}"
flock -n 9 || {
  echo "Another PACT deployment is already running." >&2
  exit 1
}

deployment_file="${incoming_dir}/deployment.env"
source_archive="${incoming_dir}/source.tar.gz"
compose_source="${incoming_dir}/docker-compose.prod.yml"

for required_file in "${deployment_file}" "${source_archive}" "${compose_source}"; do
  [[ -f "${required_file}" ]] || {
    echo "Missing deployment artifact: ${required_file}" >&2
    exit 1
  }
done

if grep -Ev '^(PACT_RELEASE_ID|PACT_SOURCE_SHA256|PACT_BUILD_DATE|PACT_GIT_COMMIT)=[A-Za-z0-9._:+-]+$' "${deployment_file}" | grep -q .; then
  echo "The deployment metadata contains an invalid line." >&2
  exit 1
fi

metadata_value() {
  local key="$1"
  local matches
  matches="$(grep -c "^${key}=" "${deployment_file}")"
  [[ "${matches}" == 1 ]] || {
    echo "Deployment metadata must contain exactly one ${key}." >&2
    exit 1
  }
  sed -n "s/^${key}=//p" "${deployment_file}"
}

PACT_RELEASE_ID="$(metadata_value PACT_RELEASE_ID)"
PACT_SOURCE_SHA256="$(metadata_value PACT_SOURCE_SHA256)"
PACT_BUILD_DATE="$(metadata_value PACT_BUILD_DATE)"
PACT_GIT_COMMIT="$(metadata_value PACT_GIT_COMMIT)"

[[ "${PACT_RELEASE_ID:-}" =~ ^[0-9]{14}-[a-f0-9]{12}$ ]] || {
  echo "Invalid PACT_RELEASE_ID." >&2
  exit 1
}
[[ "${PACT_SOURCE_SHA256:-}" =~ ^[a-f0-9]{64}$ ]] || {
  echo "Invalid PACT_SOURCE_SHA256." >&2
  exit 1
}
[[ "${PACT_BUILD_DATE:-}" =~ ^[0-9TZ:-]+$ ]] || {
  echo "Invalid PACT_BUILD_DATE." >&2
  exit 1
}
[[ "${PACT_GIT_COMMIT:-}" =~ ^[A-Za-z0-9._+-]+$ ]] || {
  echo "Invalid PACT_GIT_COMMIT." >&2
  exit 1
}
[[ "${incoming_dir}" == "/tmp/the-pact-${PACT_RELEASE_ID}" ]] || {
  echo "Unexpected incoming directory." >&2
  exit 1
}

actual_sha256="$(sha256sum "${source_archive}" | awk '{print $1}')"
[[ "${actual_sha256}" == "${PACT_SOURCE_SHA256}" ]] || {
  echo "Source archive checksum mismatch." >&2
  exit 1
}

available_kib="$(df --output=avail /var/lib/docker | tail -n 1 | tr -d ' ')"
if (( available_kib < 6291456 )); then
  echo "At least 6 GiB free are required in /var/lib/docker." >&2
  exit 1
fi

release_dir="${releases_dir}/${PACT_RELEASE_ID}"
if [[ -e "${release_dir}" ]]; then
  echo "Release already exists: ${PACT_RELEASE_ID}" >&2
  exit 1
fi

mkdir -p "${release_dir}/source"
tar -xzf "${source_archive}" -C "${release_dir}/source"
install -m 640 "${compose_source}" "${release_dir}/docker-compose.prod.yml"
install -m 640 "${deployment_file}" "${release_dir}/deployment.env"

[[ -f "${release_dir}/source/Dockerfile" && -f "${release_dir}/source/go.mod" ]] || {
  echo "The source archive does not contain a PACT project." >&2
  exit 1
}

secrets_file="${shared_dir}/runtime.env"
if [[ ! -f "${secrets_file}" ]]; then
  umask 077
  db_password="$(openssl rand -hex 32)"
  api_token="$(openssl rand -hex 32)"
  cat >"${secrets_file}" <<EOF
PACT_DB_NAME=pact
PACT_DB_USER=pact
PACT_DB_PASSWORD=${db_password}
PACT_LOCAL_API_TOKEN=${api_token}
PACT_LOCAL_ORGANIZATION_ID=00000000-0000-4000-8000-000000000001
PACT_LOG_LEVEL=info
PACT_BACKUP_RETENTION_DAYS=30
PACT_GITHUB_API_URL=https://api.github.com
PACT_GITHUB_TOKEN=
PACT_GITHUB_TIMEOUT=10s
PACT_GITHUB_SYNC_INTERVAL=0s
EOF
fi
chmod 600 "${secrets_file}"

image="the-pact-server:${PACT_RELEASE_ID}"
cat >"${release_dir}/release.env" <<EOF
PACT_SERVER_IMAGE=${image}
PACT_VERSION=${PACT_RELEASE_ID}
PACT_COMMIT=${PACT_GIT_COMMIT}
PACT_BUILD_DATE=${PACT_BUILD_DATE}
EOF
chmod 640 "${release_dir}/release.env"

docker network inspect nuanzs-edge >/dev/null 2>&1 || docker network create nuanzs-edge >/dev/null
docker buildx inspect "${builder_name}" >/dev/null 2>&1 || \
  docker buildx create --name "${builder_name}" --driver docker-container >/dev/null
docker buildx inspect "${builder_name}" --bootstrap >/dev/null

docker buildx build \
  --builder "${builder_name}" \
  --target test \
  --build-arg "GO_TEST_FLAGS=" \
  --output type=cacheonly \
  "${release_dir}/source"

integration_suffix="${PACT_SOURCE_SHA256:0:12}"
integration_database_container="pact-it-db-${integration_suffix}"
integration_network="pact-it-${integration_suffix}"
integration_image="the-pact-integration:${PACT_RELEASE_ID}"
docker network create "${integration_network}" >/dev/null
docker run \
  --detach \
  --name "${integration_database_container}" \
  --network "${integration_network}" \
  --network-alias postgres \
  --memory 512m \
  --cpus 0.25 \
  --tmpfs /var/lib/postgresql:rw,noexec,nosuid,size=384m \
  --env POSTGRES_DB=pact_test \
  --env POSTGRES_USER=pact \
  --env POSTGRES_PASSWORD=pact-integration-password \
  --env PGDATA=/var/lib/postgresql/18/docker \
  pgvector/pgvector:0.8.2-pg18-trixie@sha256:b7337db8fe39d12fe8ecb0003c72680f24479813a744b43154eee6f2eab5a5f3 \
  >/dev/null

integration_database_ready=false
for _ in {1..30}; do
  if docker exec "${integration_database_container}" pg_isready --username=pact --dbname=pact_test >/dev/null 2>&1; then
    integration_database_ready=true
    break
  fi
  sleep 2
done
[[ "${integration_database_ready}" == true ]] || {
  docker logs "${integration_database_container}" >&2 || true
  echo "The temporary PACT integration database did not become ready." >&2
  exit 1
}

docker buildx build \
  --builder "${builder_name}" \
  --target source \
  --tag "${integration_image}" \
  --load \
  "${release_dir}/source"
docker run \
  --rm \
  --network "${integration_network}" \
  --memory 1g \
  --cpus 0.50 \
  --env PACT_TEST_DATABASE_URL=postgres://pact:pact-integration-password@postgres:5432/pact_test?sslmode=disable \
  "${integration_image}" \
  go test -tags=integration -p=1 \
    ./internal/projects \
    ./internal/access \
    ./internal/agentsession \
    ./internal/coordination \
    ./internal/workspaces \
    ./internal/knowledge \
    ./internal/contextpack \
    ./internal/repositorysync
cleanup_integration
integration_database_container=""
integration_network=""
integration_image=""

docker buildx build \
  --builder "${builder_name}" \
  --target runtime \
  --build-arg "VERSION=${PACT_RELEASE_ID}" \
  --build-arg "COMMIT=${PACT_GIT_COMMIT}" \
  --build-arg "BUILD_DATE=${PACT_BUILD_DATE}" \
  --tag "${image}" \
  --load \
  "${release_dir}/source"

compose=(
  docker compose
  --project-name the-pact
  --env-file "${secrets_file}"
  --env-file "${release_dir}/release.env"
  --file "${release_dir}/docker-compose.prod.yml"
)

"${compose[@]}" config --quiet
"${compose[@]}" up --detach --wait postgres
"${compose[@]}" run --rm migrate
"${compose[@]}" up --detach --remove-orphans pact-server backup

healthy=false
for _ in {1..30}; do
  if "${compose[@]}" exec -T pact-server wget --quiet --spider http://127.0.0.1:8080/readyz; then
    healthy=true
    break
  fi
  sleep 2
done

if [[ "${healthy}" != true ]]; then
  "${compose[@]}" ps >&2 || true
  "${compose[@]}" logs --tail 100 pact-server >&2 || true
  echo "PACT did not become healthy; release was not activated." >&2
  exit 1
fi

ln -sfn "${release_dir}" "${runtime_root}/current.next"
mv -Tf "${runtime_root}/current.next" "${runtime_root}/current"

mapfile -t release_tags < <(docker image ls --format '{{.Repository}}:{{.Tag}}' --filter reference='the-pact-server:*')
mapfile -t retained_tags < <(printf '%s\n' "${release_tags[@]}" | sort -r | head -n 2)
for tag in "${release_tags[@]}"; do
  if [[ " ${retained_tags[*]} " != *" ${tag} "* ]]; then
    docker image rm "${tag}" >/dev/null 2>&1 || true
  fi
done

mapfile -t old_releases < <(find "${releases_dir}" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort -r | tail -n +11)
for old_release in "${old_releases[@]}"; do
  rm -rf -- "${releases_dir:?}/${old_release}"
done

docker container prune --force --filter label=com.docker.compose.project=the-pact >/dev/null
docker buildx prune --builder "${builder_name}" --force --filter until=168h >/dev/null || true
docker buildx prune --builder "${builder_name}" --force --keep-storage 1GB >/dev/null || true
docker buildx stop "${builder_name}" >/dev/null 2>&1 || true
rm -rf -- "${incoming_dir}"

echo "PACT release ${PACT_RELEASE_ID} is active."
