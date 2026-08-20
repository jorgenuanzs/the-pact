#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
workflow="deploy-production.yml"
branch="main"

fail() {
  printf 'PACT production deploy: %s\n' "$*" >&2
  exit 1
}

for command_name in gh git; do
  command -v "${command_name}" >/dev/null 2>&1 || fail "missing required command: ${command_name}"
done

git -C "${project_dir}" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "not inside the PACT repository"
[[ -z "$(git -C "${project_dir}" status --porcelain)" ]] || fail "the working tree has uncommitted changes; commit and push them first"

current_branch="$(git -C "${project_dir}" branch --show-current)"
[[ "${current_branch}" == "${branch}" ]] || fail "switch to ${branch} before deploying (current: ${current_branch:-detached})"

gh auth status >/dev/null 2>&1 || fail "GitHub CLI is not authenticated; run: gh auth login"
git -C "${project_dir}" fetch --quiet origin "${branch}"

local_revision="$(git -C "${project_dir}" rev-parse HEAD)"
remote_revision="$(git -C "${project_dir}" rev-parse "origin/${branch}")"
[[ "${local_revision}" == "${remote_revision}" ]] || fail "local ${branch} and origin/${branch} differ; pull or push before deploying"

repository="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
printf 'Dispatching PACT production deployment for %s at %s...\n' "${branch}" "${local_revision:0:12}"
gh workflow run "${workflow}" --repo "${repository}" --ref "${branch}"

printf 'Deployment queued. It will continue in GitHub Actions:\n'
printf 'https://github.com/%s/actions/workflows/%s\n' "${repository}" "${workflow}"
