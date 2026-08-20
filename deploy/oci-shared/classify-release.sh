#!/usr/bin/env bash
set -Eeuo pipefail

current_source="${1:-}"
candidate_source="${2:-}"
changed_paths_file="${3:-}"

if [[ -z "${current_source}" || -z "${candidate_source}" || -z "${changed_paths_file}" ]]; then
  echo "Usage: classify-release.sh CURRENT_SOURCE CANDIDATE_SOURCE CHANGED_PATHS_FILE" >&2
  exit 2
fi
if [[ ! -d "${current_source}" || ! -d "${candidate_source}" ]]; then
  echo "Both release source directories must exist." >&2
  exit 2
fi

temporary_dir="$(mktemp -d)"
cleanup() {
  rm -rf -- "${temporary_dir}"
}
trap cleanup EXIT

write_manifest() {
  local root="$1"
  local output="$2"
  local file
  local relative
  local checksum

  : >"${output}"
  while IFS= read -r -d '' file; do
    relative="${file#"${root}/"}"
    checksum="$(sha256sum "${file}" | awk '{print $1}')"
    printf '%s\t%s\n' "${checksum}" "${relative}" >>"${output}"
  done < <(find "${root}" -type f -print0 | sort -z)
}

current_manifest="${temporary_dir}/current.tsv"
candidate_manifest="${temporary_dir}/candidate.tsv"
write_manifest "${current_source}" "${current_manifest}"
write_manifest "${candidate_source}" "${candidate_manifest}"

declare -A current_hashes=()
declare -A candidate_hashes=()
declare -A all_paths=()

while IFS=$'\t' read -r checksum relative; do
  [[ -n "${relative}" ]] || continue
  current_hashes["${relative}"]="${checksum}"
  all_paths["${relative}"]=1
done <"${current_manifest}"

while IFS=$'\t' read -r checksum relative; do
  [[ -n "${relative}" ]] || continue
  candidate_hashes["${relative}"]="${checksum}"
  all_paths["${relative}"]=1
done <"${candidate_manifest}"

: >"${changed_paths_file}"
for relative in "${!all_paths[@]}"; do
  if [[ "${current_hashes[${relative}]-}" != "${candidate_hashes[${relative}]-}" ]]; then
    printf '%s\n' "${relative}" >>"${changed_paths_file}"
  fi
done
sort -o "${changed_paths_file}" "${changed_paths_file}"

profile="frontend"
if [[ ! -s "${changed_paths_file}" ]]; then
  profile="full"
else
  while IFS= read -r relative; do
    case "${relative}" in
      internal/transport/httpapi/adminui/* | internal/transport/httpapi/publicui/* | web/*) ;;
      *)
        profile="full"
        break
        ;;
    esac
  done <"${changed_paths_file}"
fi

printf '%s\n' "${profile}"
