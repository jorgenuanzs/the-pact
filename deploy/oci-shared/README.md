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

The normal production path is asynchronous. From a clean, synchronized `main`
branch, push an auditable deployment tag and return immediately:

```sh
./scripts/deploy-production.sh
# or: make deploy-production
```

The script uses the existing Git remote credential; it does not require an
active GitHub CLI session. The workflow accepts only `main` or deployment tags
created from it, serializes production deployments, builds an immutable release
on GitHub, activates it through the existing rollback-safe server script, and
verifies `https://pact.nuanzs.com/readyz`. A dedicated
self-hosted runner on the PACT VM receives this job over outbound HTTPS and
deploys locally. GitHub stores no SSH credential and the VM does not expose SSH
to GitHub-hosted runner ranges. The runner carries the additional
`pact-production` label so normal CI jobs cannot select it accidentally.

The direct deployment command remains available as a break-glass path. It
deliberately packages the working tree, but never includes `.git`, `.env`,
`.pact`, Terraform state, secrets, or local build output.

```sh
./deploy/oci-shared/deploy.sh
```

Set `PACT_DEPLOY_HOST` and `PACT_SSH_KEY` explicitly before deploying. Set
`PACT_SSH_KNOWN_HOSTS` as well when the deployment must use a dedicated pinned
host-key file. `PACT_DEPLOY_LOCAL=true` is reserved for the production runner
already operating on the target VM. No production host or private-key path is
stored in this repository or in GitHub Actions.
New runtime settings are added to an existing `runtime.env` with safe defaults;
configured values and secrets are never replaced.
The server keeps the two newest PACT images and ten newest source releases. It
prunes only the dedicated `the-pact-builder-prod` cache and PACT-labelled
containers; PostgreSQL volumes are never removed automatically.

### Automatic validation profiles

The server compares every candidate release with the active release before it
changes the `current` symlink:

- `frontend`: selected only when every changed file is under `web/` or
  `internal/transport/httpapi/adminui/`. It runs TypeScript checks, Vitest
  component tests, the Vite production build, Go formatting, focused vetting,
  and focused UI package tests. It skips the temporary integration database
  and migrations.
- `full`: used for the first deployment, for an unchanged candidate, or when
  any API, dependency, infrastructure, migration, deployment, or other source
  file changed. It retains the complete unit and PostgreSQL integration suite.

Both profiles still build the production server image, validate Compose,
start PostgreSQL, recreate the server, wait for the health checks, and preserve
the previous release for rollback. The selected profile and exact changed
paths are stored in each release directory. The first deployment containing
this classifier is necessarily a full deployment; later UI-only releases can
use the fast path.

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
