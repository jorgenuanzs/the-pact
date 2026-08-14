#!/usr/bin/env sh
set -eu

repository="jorgenuanzs/the-pact"
requested_version="${PACT_VERSION:-latest}"
install_directory="${PACT_INSTALL_DIR:-${HOME}/.local/bin}"

case "$(uname -s)" in
  Darwin) operating_system="darwin" ;;
  Linux) operating_system="linux" ;;
  *)
    echo "Pact supports this installer on macOS and Linux only." >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64|aarch64) architecture="arm64" ;;
  x86_64|amd64) architecture="amd64" ;;
  *)
    echo "Pact does not publish a binary for architecture $(uname -m)." >&2
    exit 1
    ;;
esac

if [ "$requested_version" = "latest" ]; then
  release_base="https://github.com/${repository}/releases/latest/download"
else
  case "$requested_version" in
    v*) release_tag="$requested_version" ;;
    *) release_tag="v${requested_version}" ;;
  esac
  release_base="https://github.com/${repository}/releases/download/${release_tag}"
fi

asset_name="pact_${operating_system}_${architecture}.tar.gz"
temporary_directory="$(mktemp -d)"
archive_path="${temporary_directory}/${asset_name}"
checksums_path="${temporary_directory}/checksums.txt"

cleanup() {
  rm -rf -- "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

for required_command in curl tar; do
  command -v "$required_command" >/dev/null 2>&1 || {
    echo "Missing required command: $required_command" >&2
    exit 1
  }
done

curl --fail --location --retry 3 --retry-all-errors \
  --output "$archive_path" \
  "${release_base}/${asset_name}"
curl --fail --location --retry 3 --retry-all-errors \
  --output "$checksums_path" \
  "${release_base}/checksums.txt"

expected_hash="$(awk -v asset="$asset_name" '$2 == asset || $2 == "*" asset { print $1 }' "$checksums_path")"
if [ -z "$expected_hash" ]; then
  echo "The release does not contain a checksum for ${asset_name}." >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  actual_hash="$(shasum -a 256 "$archive_path" | awk '{ print $1 }')"
elif command -v sha256sum >/dev/null 2>&1; then
  actual_hash="$(sha256sum "$archive_path" | awk '{ print $1 }')"
else
  echo "Missing SHA-256 tool: install shasum or sha256sum." >&2
  exit 1
fi

if [ "$expected_hash" != "$actual_hash" ]; then
  echo "Checksum mismatch for ${asset_name}; nothing was installed." >&2
  exit 1
fi

tar -xzf "$archive_path" -C "$temporary_directory"
test -f "${temporary_directory}/pact" || {
  echo "The Pact release archive does not contain the pact binary." >&2
  exit 1
}

mkdir -p "$install_directory"
install -m 0755 "${temporary_directory}/pact" "${install_directory}/pact"
"${install_directory}/pact" version
echo "Pact was installed at ${install_directory}/pact"
