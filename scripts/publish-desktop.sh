#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
branch="main"

fail() {
  printf 'PACT release: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: ./scripts/publish-desktop.sh patch|minor|major|vX.Y.Z [--dry-run]

Creates and pushes one stable PACT tag from the current origin/main commit.
GitHub Actions then builds Desktop, CLI, and PACT Server and publishes the
release without keeping this terminal attached.
EOF
}

next_version() {
  local current="$1"
  local bump="$2"
  [[ "${current}" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || return 1
  local major="${BASH_REMATCH[1]}"
  local minor="${BASH_REMATCH[2]}"
  local patch="${BASH_REMATCH[3]}"
  case "${bump}" in
    patch) patch=$((patch + 1)) ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    major) major=$((major + 1)); minor=0; patch=0 ;;
    *) return 1 ;;
  esac
  printf 'v%d.%d.%d\n' "${major}" "${minor}" "${patch}"
}

version_is_greater() {
  local candidate="${1#v}"
  local current="${2#v}"
  local candidate_major candidate_minor candidate_patch
  local current_major current_minor current_patch
  IFS=. read -r candidate_major candidate_minor candidate_patch <<<"${candidate}"
  IFS=. read -r current_major current_minor current_patch <<<"${current}"
  ((candidate_major > current_major)) && return 0
  ((candidate_major < current_major)) && return 1
  ((candidate_minor > current_minor)) && return 0
  ((candidate_minor < current_minor)) && return 1
  ((candidate_patch > current_patch))
}

github_repository() {
  local remote_url repository
  remote_url="$(git -C "${project_dir}" remote get-url origin)"
  case "${remote_url}" in
    https://github.com/*) repository="${remote_url#https://github.com/}" ;;
    git@github.com:*) repository="${remote_url#git@github.com:}" ;;
    ssh://git@github.com/*) repository="${remote_url#ssh://git@github.com/}" ;;
    *) fail "origin must point to a GitHub repository" ;;
  esac
  printf '%s\n' "${repository%.git}"
}

latest_stable_tag() {
  local candidate
  while IFS= read -r candidate; do
    if [[ "${candidate}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done < <(git -C "${project_dir}" tag --list 'v*' --sort=-v:refname)
  return 1
}

main() {
  local requested="${1:-}"
  local dry_run=false
  [[ -n "${requested}" ]] || { usage >&2; exit 2; }
  if [[ "${requested}" == "-h" || "${requested}" == "--help" ]]; then
    usage
    return 0
  fi
  shift
  while (($# > 0)); do
    case "$1" in
      --dry-run) dry_run=true ;;
      -h|--help) usage; return 0 ;;
      *) usage >&2; fail "unknown option: $1" ;;
    esac
    shift
  done

  for command_name in git; do
    command -v "${command_name}" >/dev/null 2>&1 || fail "missing required command: ${command_name}"
  done

  git -C "${project_dir}" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "not inside the PACT repository"
  [[ -z "$(git -C "${project_dir}" status --porcelain)" ]] || fail "the working tree has uncommitted changes; commit and push them first"
  [[ "$(git -C "${project_dir}" branch --show-current)" == "${branch}" ]] || fail "switch to ${branch} before publishing"

  git -C "${project_dir}" fetch --quiet --tags origin "${branch}"

  local local_revision remote_revision latest version
  local_revision="$(git -C "${project_dir}" rev-parse HEAD)"
  remote_revision="$(git -C "${project_dir}" rev-parse "origin/${branch}")"
  [[ "${local_revision}" == "${remote_revision}" ]] || fail "local ${branch} and origin/${branch} differ; pull or push before publishing"
  latest="$(latest_stable_tag)" || fail "no stable vX.Y.Z tag exists"

  case "${requested}" in
    patch|minor|major)
      version="$(next_version "${latest}" "${requested}")" || fail "could not calculate the next version"
      ;;
    v[0-9]*.[0-9]*.[0-9]*|[0-9]*.[0-9]*.[0-9]*)
      version="${requested#v}"
      [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "explicit versions must use vX.Y.Z"
      version="v${version}"
      version_is_greater "${version}" "${latest}" || fail "${version} must be newer than ${latest}"
      ;;
    *) usage >&2; fail "expected patch, minor, major, or vX.Y.Z" ;;
  esac

  git -C "${project_dir}" rev-parse --verify --quiet "refs/tags/${version}" >/dev/null && fail "tag ${version} already exists"

  printf 'PACT release %s\n' "${version}"
  printf '  source: %s at %s\n' "${branch}" "${local_revision:0:12}"
  printf '  previous stable release: %s\n' "${latest}"
  if [[ "${dry_run}" == true ]]; then
    printf 'Dry run complete; no tag was created or pushed.\n'
    return 0
  fi

  git -C "${project_dir}" tag -a "${version}" -m "PACT ${version}"
  if ! git -C "${project_dir}" push origin "${version}"; then
    fail "push failed; local tag ${version} was kept so the operation can be inspected safely"
  fi

  local repository
  repository="$(github_repository)"
  printf 'Release queued. It will continue in GitHub Actions:\n'
  printf 'https://github.com/%s/actions/workflows/release-cli.yml\n' "${repository}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
