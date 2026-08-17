# PACT on the shared OCI VM

This deployment shares only the HTTPS gateway and the VM with Magi. PACT owns
its Compose project, PostgreSQL instance, private network, persistent volumes,
secrets, backups, image names, and Buildx cache.

## Layout on the VM

- `/opt/the-pact/shared/runtime.env`: generated production secrets (mode `0600`).
- `/opt/the-pact/releases/<id>`: immutable application releases.
- `/opt/the-pact/current`: active release symlink.
- `the-pact_postgres-data`: PostgreSQL data volume.
- `the-pact_postgres-backups`: daily local backups, retained for 30 days.
- `nuanzs-edge`: the only Docker network shared with the HTTPS gateway.

No PACT database or HTTP port is published on the host. Caddy reaches
`pact-server:8080` through `nuanzs-edge`.

The `the-pact-gateway-reconcile.timer` unit reconnects Caddy and reapplies the
small managed PACT route after a Magi gateway deployment. It does not copy
secrets or connect either application's private network.

The hosted backoffice address is `https://pact.nuanzs.com/admin/`. DNS and the
VM address are managed outside this repository.

## Deploy

The deployment deliberately packages the working tree, because early PACT work
may not yet be committed. It never includes `.git`, `.env`, `.pact`, Terraform
state, or local build output.

```sh
./deploy/oci-shared/deploy.sh
```

Set `PACT_DEPLOY_HOST` and `PACT_SSH_KEY` explicitly before deploying. No
production host or private-key path is stored in this repository.
New runtime settings are added to an existing `runtime.env` with safe defaults;
configured values and secrets are never replaced.
The server keeps the two newest PACT images and ten newest source releases. It
prunes only the dedicated `the-pact-builder-prod` cache and PACT-labelled
containers; PostgreSQL volumes are never removed automatically.

The shared gateway integration is installed once by copying the five gateway
artifacts to the VM and running `install-gateway.sh` as root. A normal PACT
application deployment never edits or restarts Caddy.

## Operations

```sh
ssh -i "$PACT_SSH_KEY" "$PACT_DEPLOY_HOST"
sudo docker compose --project-name the-pact \
  --env-file /opt/the-pact/shared/runtime.env \
  --env-file /opt/the-pact/current/release.env \
  --file /opt/the-pact/current/docker-compose.prod.yml ps
```

The one-time `PACT_SETUP_TOKEN` can be read only by root from
`/opt/the-pact/shared/runtime.env`; remove it after the first owner account is
created. It is not an API credential. Backups are local protection against logical
errors; a later step must replicate them outside this VM for disaster recovery.

## Restore drill

Stop the application, restore a selected custom-format dump into a fresh
database with `pg_restore`, run migrations, and only then point the application
at the restored database. Do not overwrite the production volume as the first
step of a restore drill.
