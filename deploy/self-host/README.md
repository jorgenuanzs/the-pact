# Self-host PACT Server

This bundle runs the same PACT Server used by Desktop local mode, with
PostgreSQL + pgvector, migrations, health checks, durable volumes, and daily
database backups.

## Requirements

- Docker Engine or Docker Desktop
- Docker Compose v2
- 2 GB RAM minimum; 4 GB recommended for a small team
- a reverse proxy with HTTPS for any non-local deployment

## Install on a VM

1. Copy `.env.example` to `.env`.
2. Pin `PACT_SERVER_IMAGE` to the release version you downloaded.
3. Set `PACT_PUBLIC_URL` to the final HTTPS URL.
4. Generate separate values for `PACT_DB_PASSWORD` and `PACT_SETUP_TOKEN`:

   ```sh
   openssl rand -hex 32
   ```

5. Start the stack:

   ```sh
   docker compose pull
   docker compose up --detach --wait
   ```

6. Proxy the public hostname to `127.0.0.1:8080`, open `/admin/`, and create
   the first owner with the one-time setup code.
7. Clear `PACT_SETUP_TOKEN` in `.env` after setup and apply the change:

   ```sh
   docker compose up --detach
   ```

The application port is deliberately bound to loopback. Do not expose the
container directly to the internet without TLS.

## Upgrade

Create a backup, change the pinned image tag, then recreate the stack:

```sh
docker compose exec -T postgres pg_dump -U pact -d pact -Fc > pact-before-upgrade.dump
docker compose pull
docker compose up --detach --wait
```

The one-shot `migrate` service applies schema migrations before the new server
becomes healthy.

## Backup and restore

Daily backups live in the `postgres-backups` Docker volume. To create an
explicit host-side backup:

```sh
docker compose exec -T postgres pg_dump -U pact -d pact -Fc > pact.dump
```

Restoring replaces current database contents. Stop PACT clients first and keep
the source dump outside Docker:

```sh
docker compose stop pact-server backup
docker compose exec -T postgres pg_restore -U pact -d pact --clean --if-exists --no-owner < pact.dump
docker compose start pact-server backup
```

## Local personal mode

For one computer, install the PACT CLI or Desktop and use the managed flow:

```sh
pact server install
```

It creates an equivalent private stack under the operating system's PACT
configuration directory and exposes backup, upgrade, start, stop, and status
operations through both CLI and Desktop.
