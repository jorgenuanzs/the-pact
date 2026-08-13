#!/usr/bin/env bash
set -Eeuo pipefail

source_dir="${1:-}"
shared_dir="/opt/the-pact/shared"

if [[ -z "${source_dir}" || ! -d "${source_dir}" ]]; then
  echo "Usage: install-gateway.sh <artifact-directory>" >&2
  exit 2
fi

install -d -m 750 "${shared_dir}"
install -m 640 "${source_dir}/pact.Caddyfile" "${shared_dir}/pact.Caddyfile"
install -m 750 "${source_dir}/reconcile-gateway.sh" "${shared_dir}/reconcile-gateway.sh"
install -m 644 "${source_dir}/the-pact-gateway-reconcile.service" /etc/systemd/system/the-pact-gateway-reconcile.service
install -m 644 "${source_dir}/the-pact-gateway-reconcile.timer" /etc/systemd/system/the-pact-gateway-reconcile.timer

systemctl daemon-reload
systemctl enable --now the-pact-gateway-reconcile.timer
systemctl start the-pact-gateway-reconcile.service
