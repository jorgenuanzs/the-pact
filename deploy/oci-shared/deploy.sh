#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/../.." && pwd)"
deploy_host="${PACT_DEPLOY_HOST:-}"
ssh_key="${PACT_SSH_KEY:-}"
ssh_known_hosts="${PACT_SSH_KNOWN_HOSTS:-}"
temporary_dir="$(mktemp -d)"

cleanup() {
  rm -rf -- "${temporary_dir}"
}
trap cleanup EXIT

[[ -n "${deploy_host}" ]] || {
  echo "PACT_DEPLOY_HOST is required, for example ubuntu@server.example.com." >&2
  exit 1
}
[[ -n "${ssh_key}" ]] || {
  echo "PACT_SSH_KEY is required and must point to a private SSH key." >&2
  exit 1
}

for command_name in git scp shasum ssh tar; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "Missing required command: ${command_name}" >&2
    exit 1
  }
done
[[ -f "${ssh_key}" ]] || {
  echo "SSH key not found: ${ssh_key}" >&2
  exit 1
}

ssh_options=(-i "${ssh_key}")
if [[ -n "${ssh_known_hosts}" ]]; then
  [[ -f "${ssh_known_hosts}" ]] || {
    echo "SSH known-hosts file not found: ${ssh_known_hosts}" >&2
    exit 1
  }
  ssh_options+=(
    -o "UserKnownHostsFile=${ssh_known_hosts}"
    -o StrictHostKeyChecking=yes
  )
fi

source_archive="${temporary_dir}/source.tar.gz"
COPYFILE_DISABLE=1 tar \
  --no-xattrs \
  --no-mac-metadata \
  --exclude='./.git' \
  --exclude='./.DS_Store' \
  --exclude='*/.DS_Store' \
  --exclude='./.env' \
  --exclude='./.pact' \
  --exclude='./bin' \
  --exclude='./build' \
  --exclude='./coverage' \
  --exclude='./web/node_modules' \
  --exclude='./web/coverage' \
  --exclude='./web/.vite' \
  --exclude='./web/*.tsbuildinfo' \
  --exclude='./desktop/build' \
  --exclude='./desktop/frontend/dist' \
  --exclude='./desktop/frontend/wailsjs' \
  --exclude='./desktop/frontend/package.json.md5' \
  --exclude='./desktop/localhelper/pact-local' \
  --exclude='./desktop/localhelper/pact-local.exe' \
  --exclude='./internal/transport/httpapi/adminui/dist' \
  --exclude='./internal/transport/httpapi/publicui/dist' \
  --exclude='./dist' \
  --exclude='./infra/secrets' \
  --exclude='./infra/oci-madrid/.terraform' \
  --exclude='./infra/oci-madrid/terraform.tfstate*' \
  --exclude='./infra/oci-madrid/*.tfvars' \
  --exclude='./tmp' \
  -czf "${source_archive}" \
  -C "${project_dir}" \
  .

source_sha256="$(shasum -a 256 "${source_archive}" | awk '{print $1}')"
release_id="$(date -u +%Y%m%d%H%M%S)-${source_sha256:0:12}"
build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
git_commit="$(git -C "${project_dir}" rev-parse --short HEAD)"
if [[ -n "$(git -C "${project_dir}" status --porcelain)" ]]; then
  git_commit="${git_commit}-dirty"
fi

cat >"${temporary_dir}/deployment.env" <<EOF
PACT_RELEASE_ID=${release_id}
PACT_SOURCE_SHA256=${source_sha256}
PACT_BUILD_DATE=${build_date}
PACT_GIT_COMMIT=${git_commit}
EOF
cp "${script_dir}/docker-compose.prod.yml" "${temporary_dir}/docker-compose.prod.yml"
cp "${script_dir}/activate-release.sh" "${temporary_dir}/activate-release.sh"
chmod 700 "${temporary_dir}/activate-release.sh"

remote_dir="/tmp/the-pact-${release_id}"
ssh "${ssh_options[@]}" "${deploy_host}" "mkdir -m 700 '${remote_dir}'"
scp "${ssh_options[@]}" \
  "${source_archive}" \
  "${temporary_dir}/deployment.env" \
  "${temporary_dir}/docker-compose.prod.yml" \
  "${temporary_dir}/activate-release.sh" \
  "${deploy_host}:${remote_dir}/"
ssh "${ssh_options[@]}" "${deploy_host}" "sudo bash '${remote_dir}/activate-release.sh' '${remote_dir}'"

echo "Deployed PACT release ${release_id} to ${deploy_host}."
