# Pact

[![CI](https://github.com/jorgenuanzs/the-pact/actions/workflows/ci.yml/badge.svg)](https://github.com/jorgenuanzs/the-pact/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/jorgenuanzs/the-pact)](https://github.com/jorgenuanzs/the-pact/releases/latest)
[![License](https://img.shields.io/github/license/jorgenuanzs/the-pact)](LICENSE)

**Live coordination and shared project context for people and AI agents.**

Pact is an open-source control plane for projects where humans and AI agents
work on the same codebase. Git remains the source of truth for files and
history; Pact maintains the live operational state around Git: who is working,
what they intend to change, which scopes they have reserved, what the repository
currently looks like, and what context the next participant needs.

> [!IMPORTANT]
> Pact is currently an early alpha. The implemented vertical slice is useful
> for controlled development environments, but it is not yet a hardened
> authorization boundary or a replacement for code review, CI, Git hosting, or
> an operating-system sandbox.

## Why Pact exists

Branching solved an important human coordination problem: several developers
can change one project without continuously overwriting each other's work. AI
agents increase the amount of concurrent work, but a branch alone does not tell
them:

- who else is active right now;
- what outcome another agent is pursuing;
- whether two changes overlap semantically;
- which decisions and constraints still apply;
- which machine, actor, and intention produced a change;
- whether local uncommitted work is already in progress;
- where an isolated Git worktree should be created.

Pact adds that live coordination layer without replacing Git.

```text
Git   = durable content and history
Pact  = live presence, intent, coordination, context, and evidence
```

## What works today

The current implementation includes:

- a modular Go server with a versioned HTTP API;
- PostgreSQL 18 with pgvector as the canonical data store;
- transactional state changes, durable events, an outbox, and resumable SSE;
- personal identities, project roles, one-time invitations, and revocation;
- project discovery based on normalized Git remotes;
- organization-level GitHub App installations, selected-repository access, and one-hour repository-scoped tokens;
- multi-repository projects with flexible purposes, a primary revision projection, and per-repository verified state;
- durable workspaces that group related projects under shared context;
- typed knowledge records, source references, evidence, review states, and deterministic Workspace context;
- human-created Workspace rooms with bounded chat history, replies, explicit `@` mentions, and separate inboxes for people and agents;
- structured cross-agent handoffs and immutable, verifiable Context Packs;
- machine identities, agent sessions, heartbeats, and live presence;
- privacy-preserving Git observation from Pact Node and wrapped agents;
- a local MCP server for project context and coordinated work;
- durable work intentions, hierarchical scopes, leases, and overlap detection;
- isolated Git worktrees provisioned for coordinated tasks;
- a live web backoffice for projects, active work, and recent events;
- native CLI releases for Windows, macOS, and Linux on `amd64` and `arm64`;
- Docker Compose development and production deployment examples.

Document ingestion, embedding-backed hybrid search, infrastructure
capability broker, policy engine, and semantic conflict analysis remain on the
roadmap. See [Project status and limitations](#project-status-and-limitations).

## Architecture

```text
┌──────────────────────────── Local machine ────────────────────────────┐
│                                                                      │
│  Codex / Claude / Kimi ── stdio MCP ── pact mcp serve               │
│             │                               │                        │
│             └── optional wrapper ── pact agent run                  │
│                                             │                        │
│  IDE / terminal / human changes ── pact node run                    │
│                                             │                        │
│  Git repository + .pact runtime + isolated worktrees                │
└───────────────────────────────┬──────────────────────────────────────┘
                                │ HTTPS
                                ▼
┌──────────────────────────── Pact Server ─────────────────────────────┐
│ Identity · workspaces · projects · knowledge · rooms · sessions   │
│ Intents · Handoffs · Context Packs · repositories · events        │
│  Embedded live backoffice                                           │
└───────────────────────────────┬──────────────────────────────────────┘
                         ┌──────┴──────┐
                         ▼             ▼
              PostgreSQL + pgvector  GitHub App + REST API
```

One Pact Server can host many workspaces and projects for a team. A workspace
represents a product, client, or initiative and may contain several technical
projects. Each checkout keeps only its machine-specific binding locally and
connects to the shared server. A developer does not need PostgreSQL or Docker
on their computer when using a remote Pact Server.

## Quick start: run Pact Server locally

### Requirements

- Git;
- Docker Desktop, or Docker Engine with Docker Compose v2 and Buildx;
- GNU Make;
- `curl` for the health checks.

Clone and initialize the environment:

```sh
git clone https://github.com/jorgenuanzs/the-pact.git
cd the-pact
make init
```

`make init` creates `.env` from `.env.example`. Set both blank secrets before
continuing:

```dotenv
PACT_DB_PASSWORD=<at-least-16-url-safe-characters>
PACT_LOCAL_API_TOKEN=<at-least-24-random-characters>
```

For example, `openssl rand -hex 32` generates an appropriate value. Use a
different value for each setting and never commit `.env`.

Start the server and PostgreSQL:

```sh
make dev
```

Check the service:

```sh
curl --fail-with-body http://127.0.0.1:8080/livez
curl --fail-with-body http://127.0.0.1:8080/readyz
```

Open the backoffice at [http://127.0.0.1:8080/admin/](http://127.0.0.1:8080/admin/)
and enter `PACT_LOCAL_API_TOKEN`. The UI keeps it only in the browser tab's
`sessionStorage`; it is not embedded in the page or added to the URL.

## Install the CLI

Every release contains checksummed native binaries for:

| Operating system | Architectures |
|---|---|
| Windows | `amd64`, `arm64` |
| macOS | `amd64`, `arm64` |
| Linux | `amd64`, `arm64` |

### Homebrew (macOS and Linux)

Install Pact from the official tap:

```sh
brew install jorgenuanzs/pact/pact
pact version
```

The fully qualified command automatically adds the tap and trusts only the Pact
formula. See the
[official Pact tap](https://github.com/jorgenuanzs/homebrew-pact) for upgrade,
uninstall, and `Brewfile` instructions.

### Installer script (macOS and Linux)

Download and inspect the installer, then run it:

```sh
curl -fL https://github.com/jorgenuanzs/the-pact/releases/latest/download/install-pact.sh \
  -o /tmp/install-pact.sh
sh /tmp/install-pact.sh
```

The default destination is `~/.local/bin/pact`. Override it when needed:

```sh
PACT_INSTALL_DIR=/usr/local/bin sh /tmp/install-pact.sh
```

### Windows

Git for Windows is required for project operations. Download and run the native
PowerShell installer:

```powershell
$installer = Join-Path $env:TEMP "install-pact.ps1"
Invoke-WebRequest `
  "https://github.com/jorgenuanzs/the-pact/releases/latest/download/install-pact.ps1" `
  -OutFile $installer
powershell.exe -NoProfile -ExecutionPolicy Bypass -File $installer
Remove-Item $installer
```

The installer detects `amd64` or `arm64`, verifies the published SHA-256, places
`pact.exe` in `%LOCALAPPDATA%\Programs\Pact`, and adds that directory to the
user's `PATH`.

### Build from source

With the Go version declared in `go.mod`:

```sh
go build -o ./bin/pact ./cmd/pact
```

Or use Docker without installing Go locally:

```sh
make cli
```

## Connect your first repository

Pact Server and the Pact CLI are separate. The server is shared; the CLI runs
next to each participant's Git checkout.

### 1. Log in on the first computer

For a self-hosted server, use the bootstrap token from its `.env`. Read it from
standard input so it does not appear in shell history:

```sh
printf '%s' "$PACT_API_TOKEN" | pact login \
  --server http://127.0.0.1:8080 \
  --token-stdin
```

PowerShell equivalent:

```powershell
$env:PACT_API_TOKEN | pact login `
  --server http://127.0.0.1:8080 `
  --token-stdin
```

Remote non-loopback servers must use HTTPS.

### 2. Initialize the project

Run this anywhere inside the Git repository:

```sh
cd your-repository
pact init
```

Pact discovers the Git root and remote, creates or recovers the project on the
server, and writes two different surfaces:

| Path | Purpose | Git visibility |
|---|---|---|
| `pact.yaml` | Shared project manifest | Commit it |
| `.pact/config.json` | Server URL and remote project UUID for this checkout | Ignored |
| `.pact/node.json` | Private machine identity, created when observation starts | Ignored |
| `.pact/worktrees/` | Isolated Git worktrees created for coordinated work | Ignored |

No API token, PostgreSQL credential, or cloud secret is written into the
repository.

`pact init` also creates a default remote Workspace for a new project. Related
projects can later be grouped without changing their Git repositories:

```sh
pact workspace list
pact workspace show footfall
pact workspace create \
  --name "Footfall Product" \
  --slug footfall-product \
  --project PROJECT_UUID
pact workspace add-project WORKSPACE_UUID ANOTHER_PROJECT_UUID
```

A project belongs to one Workspace. Moving it changes shared context and
navigation only; it never moves files, branches, or remotes.

### 3. Verify the identity

```sh
pact whoami
pact version
```

## Add another person or computer

The project owner creates a one-time invitation from a connected checkout:

```sh
pact invite create \
  --email collaborator@example.com \
  --role contributor
```

Available project roles are `owner`, `maintainer`, `contributor`, and `viewer`.
Invitations last 24 hours by default and may be configured between one hour and
seven days with `--expires`.

Send the displayed `pact_inv_...` secret through a private channel. It is shown
only once.

The collaborator installs Pact, clones the repository, and accepts the
invitation:

```sh
git clone https://github.com/example/project.git
cd project

printf '%s' "$PACT_INVITATION" | pact join \
  --server https://pact.example.com \
  --name "Collaborator name" \
  --invite-stdin

pact connect
```

`pact connect` requires the `pact.yaml` created by the owner and connects only
to an existing remote project. It never creates a project silently. SSH and
HTTPS Git remotes are normalized before comparison, so different clone methods
still resolve to the same Pact project.

## Connect AI agents

### Codex

From the connected repository:

```sh
pact enable codex
```

This writes a machine-local MCP block to `.codex/config.toml` and excludes that
file through `.git/info/exclude`. It does not dirty the repository or affect
another checkout. Restart Codex or reload the VS Code window before opening a
new chat.

Codex CLI, the Codex VS Code extension, and the desktop application can use the
same project-scoped configuration.

### Claude Code

From the connected repository:

```sh
pact enable claude
```

This creates an idempotent `.mcp.json` entry with the absolute local Pact and
project paths. When Pact creates the file, it excludes it through
`.git/info/exclude`, so another checkout can keep its own machine-specific
configuration. Restart Claude Code and approve the project MCP server when
prompted. Claude documents this approval boundary for project-scoped MCP
servers in its [official MCP guide](https://code.claude.com/docs/en/mcp).

### Any MCP-compatible client

Start Pact as a local `stdio` MCP server:

```json
{
  "mcpServers": {
    "pact": {
      "command": "/absolute/path/to/pact",
      "args": [
        "mcp", "serve",
        "--client", "your-client",
        "--name", "Your agent",
        "--path", "/absolute/path/to/repository"
      ]
    }
  }
}
```

The MCP client owns the process lifecycle. The computer must already be logged
in, and the checkout must have completed `pact init` or `pact connect`.

### Wrap a command-line agent

Agents without MCP integration can still publish presence and Git observations
by running through Pact:

```sh
pact agent run --client claude -- claude
pact agent run --client kimi -- kimi
pact agent run --client codex -- codex
```

Pact opens an attributed session, maintains its heartbeat, observes the Git
checkout, and closes the session when the child process exits. It does not
capture prompts, conversations, stdin, or stdout.

### Observe human and IDE changes

Run Pact Node in a separate terminal:

```sh
pact node run
```

Or capture one observation and exit:

```sh
pact node run --once
```

Pact sends only dirty/clean state, branch, HEAD revision, changed-path count,
and a SHA-256 fingerprint. File names and contents are not sent to Pact Server.

## Shared knowledge and coordinated work through MCP

The local MCP adapter exposes the following tools:

| Tool | Purpose |
|---|---|
| `pact.project_context` | Return project identity, Workspace knowledge, live work, recent events, and summarized Git state |
| `pact.list_projects` | List projects visible to the current identity |
| `pact.list_workspaces` | List shared Workspaces and their related projects |
| `pact.workspace_context` | Return accepted decisions, requirements, constraints, open questions, risks, and sources |
| `pact.rooms` | List human-created rooms and participants, read bounded context, post or reply, and handle this agent's explicit mention inbox |
| `pact.list_resources` | Search registered source references |
| `pact.add_resource` | Register a source locator without copying its content |
| `pact.list_records` | Search typed, evidence-backed knowledge records |
| `pact.propose_record` | Propose a durable knowledge record with optional evidence |
| `pact.review_record` | Accept, dispute, supersede, revoke, expire, or reject a record |
| `pact.refresh_git_observation` | Refresh the current checkout observation |
| `pact.get_repository_sync` | Read the last GitHub-verified canonical branch and commit |
| `pact.sync_repository` | Ask Pact Server to verify canonical state directly with GitHub |
| `pact.check_scopes` | Detect conflicting hierarchical reservations before work begins |
| `pact.start_work` | Create an intent, acquire scopes, and provision an isolated worktree |
| `pact.list_work` | List active and historical coordinated work |
| `pact.update_work` | Block, resume, submit, cancel, abandon, or complete work |
| `pact.list_handoffs` | List structured handoff offers and responses |
| `pact.offer_handoff` | Offer completed work, remaining work, blockers, validations, and next steps |
| `pact.update_handoff` | Accept another actor's offer or withdraw your own |
| `pact.compile_context_pack` | Persist an intent-specific snapshot with event cursor, Git revision, expiry, and source fingerprint |
| `pact.get_context_pack` | Retrieve a persisted Context Pack after its payload integrity check |

The recommended agent flow is:

```text
project_context → workspace_context → get_repository_sync → check_scopes → start_work → edit worktree_path
                → compile_context_pack → offer_handoff or update_work
```

Rooms are deliberately outside that mandatory coordination flow. A human
creates a small number of long-lived rooms and may mention an agent with `@`.
The agent then calls `pact.rooms` to inspect its inbox and read only the
relevant bounded message window. Pact never injects every room into every
prompt, and posting a message never creates an intent, scope, branch, or
worktree.

`pact.start_work` returns a private `worktree_path` created under
`.pact/worktrees/`. The deprecated `workspace_path` alias remains in the result
for v0.7 clients. Agents should edit only that worktree during coordinated work.
Exclusive overlapping scopes are rejected unless a caller explicitly requests
an override.

A Handoff is an acknowledgement protocol, not a hidden file transfer. Accepting
one does not move the sender's local worktree, session, responsibility, or scope
leases. The sender should release or close the original work, after which the
recipient starts a new coordinated intent and receives a fresh worktree.

A Context Pack is a short-lived immutable snapshot, not a conversation dump.
It includes structured project and Workspace state, relevant knowledge, live
work, Handoffs, the Git revision, an event cursor, and SHA-256 provenance. The
current compiler is deterministic and does not invoke an LLM.

Git still owns commits and branches. Pact adds intent, responsibility, leases,
and conflict signals around them.

## Live backoffice

The embedded backoffice is available at `/admin/` on Pact Server. It shows:

- visible Workspaces with their projects grouped beneath them;
- human-created context rooms, replies, participant suggestions, and personal mention inboxes;
- currently active humans and agents;
- intended work, reserved scopes, branches, and worktree status;
- accepted decisions, requirements, constraints, open questions, risks, and registered sources;
- offered, accepted, withdrawn, and expired handoffs with their blockers, next steps, and validations;
- observed dirty/clean activity;
- durable recent events in real time through SSE.

The dashboard distinguishes presence, coordinated intent, and evidence of code
changes. `unobserved` means no recent observer has reported the checkout; it
does not prove that nobody is changing code.

The backoffice is read-only with respect to Git. It does not run commands,
merge branches, or mutate repositories.

## Security and privacy model

Pact is designed around several boundaries:

- tokens are accepted through stdin or environment variables, never URL query
  parameters;
- PostgreSQL stores digests of invitations and personal access tokens;
- user credentials live outside repositories in `~/.config/pact/config.json`
  on macOS/Linux and `%APPDATA%\Pact\config.json` on Windows;
- `.pact/` contains machine-local state and is ignored by Git;
- private agent conversations, prompts, stdin, stdout, and command output are not captured; only messages deliberately posted to a Workspace room are stored;
- Git observations do not upload file names, diffs, or source contents;
- non-loopback server URLs require HTTPS;
- generated containers drop Linux capabilities and use read-only filesystems
  where practical;
- database, server, and backup volumes are never removed by routine cleanup
  commands.

Current limitations matter:

- Pact does not sandbox an AI agent. The operating system and the agent client
  still determine what local files and commands it can access;
- observer mode does not prevent someone from bypassing Pact and changing Git
  directly;
- room mentions create durable inbox items but do not wake or run an agent by themselves;
- local tokens are permission-protected files, not yet OS keychain entries;
- the bootstrap token is powerful and should be replaced by personal
  invitations for routine collaboration;
- production deployment requires HTTPS, backups, monitoring, and appropriate
  secret management.

Please report security issues privately as described in [SECURITY.md](SECURITY.md).

## CLI reference

| Command | Description |
|---|---|
| `pact login --server URL --token-stdin` | Authenticate this computer with a bootstrap or personal token |
| `pact init [PATH]` | Create or recover a project and connect the owner checkout |
| `pact connect [PATH]` | Connect another checkout to an existing Pact project |
| `pact repository list` | Show the primary and additional project repositories and their verified revisions |
| `pact repository status [--repository UUID]` | Show verified state for the primary or selected repository |
| `pact repository sync [--repository UUID]` | Verify the primary or selected repository with GitHub |
| `pact enable codex` | Install the project-scoped Codex MCP configuration |
| `pact enable claude` | Install the project-scoped Claude Code MCP configuration |
| `pact invite create --email EMAIL` | Create a one-time project invitation |
| `pact join --server URL --name NAME --invite-stdin` | Accept an invitation and store the new personal token |
| `pact whoami` | Show the current identity and server |
| `pact logout --revoke` | Revoke the current token and delete it locally |
| `pact agent run --client TYPE -- COMMAND` | Run and observe an agent process |
| `pact node run` | Continuously observe human, IDE, and external Git changes |
| `pact mcp serve --client TYPE` | Start the local MCP adapter over stdio |
| `pact version` | Print version, commit, and build metadata as JSON |

Run `pact help` or append `-h` to a subcommand for flags.

## Server configuration

The local Compose environment is documented in `.env.example`. Important
settings include:

| Variable | Purpose |
|---|---|
| `PACT_DB_PASSWORD` | PostgreSQL password; use a URL-safe random value |
| `PACT_LOCAL_API_TOKEN` | Initial bootstrap credential |
| `PACT_LOCAL_ORGANIZATION_ID` | UUID of the initial organization |
| `PACT_HTTP_PORT` | Loopback port exposed by Docker Compose |
| `PACT_DATABASE_TIMEOUT` | Database connection timeout |
| `PACT_DATABASE_STATEMENT_TIMEOUT` | PostgreSQL statement timeout |
| `PACT_DATABASE_LOCK_TIMEOUT` | PostgreSQL lock timeout |
| `PACT_SHUTDOWN_TIMEOUT` | Graceful server shutdown timeout |
| `PACT_LOG_LEVEL` | `debug`, `info`, `warn`, or `error` |
| `PACT_PUBLIC_URL` | Public HTTPS origin used by GitHub callbacks and webhooks |
| `PACT_GITHUB_API_URL` | GitHub REST API base URL; defaults to `https://api.github.com` |
| `PACT_GITHUB_WEB_URL` | GitHub web origin; defaults to `https://github.com` |
| `PACT_GITHUB_TOKEN` | Optional static fallback credential for development or GHES |
| `PACT_GITHUB_APP_ID` | Numeric GitHub App ID |
| `PACT_GITHUB_APP_SLUG` | Public slug used by the GitHub installation page |
| `PACT_GITHUB_APP_CLIENT_ID` | Client ID used for installation ownership verification |
| `PACT_GITHUB_APP_CLIENT_SECRET` | Client secret; also protects the PKCE verifier derivation |
| `PACT_GITHUB_APP_PRIVATE_KEY_BASE64` | Base64-encoded RSA PEM private key used to sign App JWTs |
| `PACT_GITHUB_APP_WEBHOOK_SECRET` | Secret used to verify GitHub webhook signatures |
| `PACT_GITHUB_TIMEOUT` | Timeout for each GitHub request |
| `PACT_GITHUB_SYNC_INTERVAL` | Automatic polling interval; `0s` disables polling |

### GitHub App setup

The recommended production integration is a GitHub App. In its settings:

1. grant only **Metadata: read-only** and **Contents: read-only** repository permissions;
2. leave **Request user authorization (OAuth) during installation** disabled;
3. set both the callback URL and setup URL to
   `https://YOUR_PACT_HOST/v1/integrations/github/callback`;
4. enable webhooks at `https://YOUR_PACT_HOST/v1/integrations/github/webhook`,
   subscribe to `installation` and `installation_repositories`, and configure a webhook secret;
5. configure every `PACT_GITHUB_APP_*` variable together and restart Pact Server.

An organization owner or administrator can then use **Connect GitHub** in
Pact Control. GitHub displays its native account and repository selector. Pact
returns through a short OAuth verification step with PKCE, proves that the
current GitHub user can access the installation, and immediately discards the
user token. Installation tokens are minted only when needed, limited to the
selected repository, cached in memory, and never stored in PostgreSQL.

A Pact project may attach any number of authorized repositories. Each link has
a flexible purpose such as `frontend`, `backend`, `mobile`, `infra`, or `docs`,
plus required/optional and primary/additional flags. The primary repository
continues to project its revision into `project.canonical_revision` for
compatibility; Context Packs also carry the complete repository revision set.

The API contract is available in [api/openapi.yaml](api/openapi.yaml).

## Development

Common commands:

| Command | Purpose |
|---|---|
| `make doctor` | Validate Docker and local configuration |
| `make dev` | Build and start the development stack |
| `make ps` | Show Pact containers |
| `make logs` | Follow server and database logs |
| `make test` | Run unit tests in the reproducible Docker build |
| `make test-race` | Run the Go race detector |
| `make test-integration` | Run PostgreSQL integration tests |
| `make build` | Build the hardened server image |
| `make verify` | Validate Compose, test, build, and clean stale artifacts |
| `make docker-audit` | Show only Pact-related containers, images, volumes, and cache |
| `make docker-clean-stale` | Remove stale Pact build artifacts while preserving volumes |
| `make down` | Stop the local stack without deleting PostgreSQL data |

Native Go checks:

```sh
go vet ./...
go test ./...
go test -race ./...
```

CI runs the complete suite on Windows, macOS, and Linux and cross-compiles both
Windows architectures. A release is smoke-tested by installing its published
artifact on clean runners before the workflow succeeds.

For a detailed setup and API walkthrough, see
[docs/development.md](docs/development.md). Architecture decisions live under
[docs/adr](docs/adr).

## Project layout

```text
cmd/pact/                 CLI, agent wrapper, Pact Node, and MCP adapter
cmd/pact-server/          server entry point and migrations command
internal/                 domain modules and infrastructure adapters
internal/transport/       HTTP API and embedded backoffice
api/openapi.yaml          versioned HTTP contract
docs/adr/                 accepted architecture decisions
docs/spec/                protocol and core-loop specifications
infra/                    infrastructure examples
deploy/                   production deployment examples
scripts/                  cross-platform installers and packaging helpers
PACT_MASTER_PLAN.md       complete long-term product and architecture vision
```

## Project status and limitations

Pact builds deterministic coordination and knowledge primitives before adding
AI-dependent retrieval or synthesis.

Implemented now:

- project and repository identity;
- access, invitations, roles, and revocation;
- live sessions and Git observation;
- MCP project context;
- typed Workspace Resources and Records with evidence, lifecycle review, full-text search, and deterministic context;
- durable Workspace rooms with manual membership in the information flow, replies, explicit mentions, and bounded MCP reads;
- coordinated intentions, scopes, leases, and worktrees;
- structured Handoffs and intent-specific Context Packs with integrity verification;
- backoffice and durable event stream;
- reproducible self-hosting and native clients.

Not implemented yet:

- OIDC, SSO, SCIM, and operating-system keychain integration;
- automatic Git merge governance or a mandatory write gateway;
- document content ingestion, transcript processing, chunking, and automatic provenance extraction;
- vector and hybrid search exposed as product capabilities;
- code graph and semantic conflict analysis;
- infrastructure inventory and delegated execution capabilities;
- cloud secret-manager mediation;
- policy-as-code approvals and protected environment runners;
- high-availability deployment and off-host backup replication;
- public hosted account registration.

The long-term design is documented in
[PACT_MASTER_PLAN.md](PACT_MASTER_PLAN.md). That document describes the target
system, not a claim that every component already exists.

## Contributing

Issues, design discussions, and pull requests are welcome. Start with
[CONTRIBUTING.md](CONTRIBUTING.md), preserve Git as the authority for files, and
keep security-sensitive behavior deterministic and auditable.

## License

Pact is licensed under the [Apache License 2.0](LICENSE).
