#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
workflow="deploy-production.yml"
branch="main"
tag_prefix="deploy-production"

fail() {
  printf 'PACT production deploy: %s\n' "$*" >&2
  exit 1
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

for command_name in git; do
  command -v "${command_name}" >/dev/null 2>&1 || fail "missing required command: ${command_name}"
done

git -C "${project_dir}" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "not inside the PACT repository"
[[ -z "$(git -C "${project_dir}" status --porcelain)" ]] || fail "the working tree has uncommitted changes; commit and push them first"

current_branch="$(git -C "${project_dir}" branch --show-current)"
[[ "${current_branch}" == "${branch}" ]] || fail "switch to ${branch} before deploying (current: ${current_branch:-detached})"

git -C "${project_dir}" fetch --quiet origin "${branch}"

local_revision="$(git -C "${project_dir}" rev-parse HEAD)"
remote_revision="$(git -C "${project_dir}" rev-parse "origin/${branch}")"
[[ "${local_revision}" == "${remote_revision}" ]] || fail "local ${branch} and origin/${branch} differ; pull or push before deploying"

repository="$(github_repository)"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
deployment_tag="${tag_prefix}-${timestamp}-${local_revision:0:12}"

printf 'Queueing PACT production deployment for %s at %s...\n' "${branch}" "${local_revision:0:12}"
printf '  deployment tag: %s\n' "${deployment_tag}"

if [[ "${PACT_DEPLOY_DRY_RUN:-false}" == "true" ]]; then
  printf 'Dry run complete; no tag was created or pushed.\n'
  exit 0
fi

git -C "${project_dir}" tag -a "${deployment_tag}" "${local_revision}" \
  -m "Deploy PACT production from ${local_revision}"
if ! git -C "${project_dir}" push origin "refs/tags/${deployment_tag}"; then
  git -C "${project_dir}" tag --delete "${deployment_tag}" >/dev/null
  fail "the deployment tag could not be pushed"
fi
git -C "${project_dir}" tag --delete "${deployment_tag}" >/dev/null

printf 'Deployment queued. It will continue in GitHub Actions:\n'
printf 'https://github.com/%s/actions/workflows/%s\n' "${repository}" "${workflow}"
