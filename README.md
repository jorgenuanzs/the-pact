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
- a native Wails desktop client for securely connecting macOS and Windows to a remote Pact Server;
- signed in-app Desktop updates with checksum and pinned Ed25519 verification;
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

## Download

The public site at [pact.nuanzs.com](https://pact.nuanzs.com/) detects the
current operating system and links to the latest Desktop and CLI artifacts.
Each release also includes SHA-256 checksums, a machine-readable manifest, and
a self-host bundle. Desktop is the recommended entry point for macOS and
Windows; the CLI remains available for terminals, automation, and Linux.

## Quick start: run Pact Server locally

The simplest personal installation is managed by Desktop, or by the CLI:

```sh
pact server install
```

PACT creates a private Docker Compose stack containing PACT Server,
PostgreSQL + pgvector, migrations, and durable storage. It prints a one-time
setup code and exposes the control plane only on `127.0.0.1:8080`. The same
installation can be started, stopped, backed up, and upgraded from Desktop or
with `pact server` commands.

### Develop the server from source

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

`make init` creates `.env` from `.env.example`. Set the database password and a
one-time setup code before continuing:

```dotenv
PACT_DB_PASSWORD=<at-least-16-url-safe-characters>
PACT_SETUP_TOKEN=<at-least-24-random-characters>
```

For example, `openssl rand -hex 32` generates an appropriate value. Use a
different value for each setting and never commit `.env`. `PACT_SETUP_TOKEN`
is accepted only while no local owner exists; it is never an API credential.

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
and create the first owner with the setup code. Then remove
`PACT_SETUP_TOKEN` from `.env` and restart the server. Subsequent visits use
the owner's username or email and password; the browser receives an HttpOnly,
SameSite session cookie instead of seeing an API credential.

## PACT Desktop

PACT Desktop wraps the same React control plane in a lightweight native shell.
The first milestone connects macOS and Windows to an existing remote Pact
Server through device authorization; the user's password remains in the server
login page and the native credential never enters the React runtime.

The desktop rail also exposes a separate **This computer** area. It manages
local checkouts, detects Codex and Claude Code, and installs project-scoped MCP
configuration through a native folder picker. The package contains its own
`pact-local` runtime, so agents do not depend on a Homebrew, PowerShell, or
standalone CLI installation and can connect while the Desktop window is closed.

Build and test it locally with:

```sh
make desktop-install
make desktop-test
make desktop-build
```

The macOS output is `desktop/build/bin/PACT.app`. The dedicated Desktop CI
workflow also creates a Windows NSIS installer. Stable distribution additionally
requires Apple Developer ID notarization and Windows Authenticode; update
archives carry a separate Ed25519 signature verified inside PACT. See
[`desktop/README.md`](desktop/README.md) for architecture, development, and
distribution notes. The same application can connect to an existing team
server or manage a local PACT Server and PostgreSQL installation. Docker
Desktop (or Docker Engine with Compose v2) is required only for local server
mode.

## Self-host for a team

Every release includes `pact-server-self-host.zip` and publishes the matching
multi-architecture container image to GitHub Container Registry. The bundle
contains a production-oriented Compose definition, `.env.example`, health
checks, durable volumes, and daily database backups. Put an HTTPS reverse proxy
in front of the loopback-bound PACT port and connect each Desktop or CLI client
to that shared URL. Full instructions live in
[`deploy/self-host/README.md`](deploy/self-host/README.md).

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

The CLI uses device authorization. It opens Pact in the browser, where you log
in and confirm the code shown by the terminal:

```sh
pact login --server http://127.0.0.1:8080
```

PowerShell equivalent:

```powershell
pact login --server http://127.0.0.1:8080
```

Remote non-loopback servers must use HTTPS.

PACT can authorize several servers on the same computer. Give a server an
optional local name and inspect or change the default preference with:

```sh
pact login --server https://pact.example.com --name "Client production"
pact servers list
pact servers use https://pact.example.com
```

`pact servers use` affects only commands executed without a connected
checkout. Inside a connected repository, `.pact/config.json` always selects
its own server and PACT retrieves that server's credential. It never falls
back to a different active profile. This allows two terminals, repositories,
or MCP clients to operate against different PACT Servers simultaneously.

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
| `.pact/config.json` | Server, workspace, repository, project and Git remote fingerprint for this checkout | Ignored |
| `.pact/node.json` | Private machine identity, created when observation starts | Ignored |
| `.pact/worktrees/` | Isolated Git worktrees created for coordinated work | Ignored |

No password, device credential, PostgreSQL credential, or cloud secret is
written into the repository. The private binding stores a SHA-256 fingerprint
of the normalized Git remote rather than the raw URL.

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
pact status
pact version
```

`pact status` reports the server profile, workspace, project, and repository
resolved for the current checkout. Use `pact status --json` for automation.

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

Send the private registration URL through a trusted channel. It contains the
one-time `pact_inv_...` invitation and is shown only once.

The collaborator opens that URL, creates an account, installs Pact, and then
authorizes the computer before connecting the checkout:

```sh
pact login --server https://pact.example.com

git clone https://github.com/example/project.git
cd project
pact connect
```

If the collaborator receives the raw invitation secret instead of the URL,
this opens the same registration screen:

```sh
printf '%s' "$PACT_INVITATION" | pact join \
  --server https://pact.example.com \
  --invite-stdin
```

`pact login` then asks the signed-in account to approve this computer and
stores a separate revocable device credential in macOS Keychain, Windows
Credential Manager, or the platform-native user keyring. `config.json` keeps
only non-secret server profile metadata.

`pact connect` requires the `pact.yaml` created by the owner and connects only
to an existing remote project. It never creates a project silently. SSH and
HTTPS Git remotes are normalized before comparison, so different clone methods
still resolve to the same Pact project. If a project or remote is visible in
more than one destination, select it explicitly with `--workspace UUID` and
`--repository UUID`.

Changing an existing checkout to another server, workspace, repository, or Git
remote is intentionally explicit:

```sh
pact connect --server https://other-pact.example.com --rebind
```

PACT validates the new destination before atomically replacing the binding.
The local node identity rotates only when the server changes. A separate Git
worktree may keep an independent binding to another server.

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
        "--path", "/absolute/path/to/repository"
      ]
    }
  }
}
```

The MCP client owns the process lifecycle. The computer must already be logged
in, and the checkout must have completed `pact init` or `pact connect`.
The durable agent identity comes from the client type and sponsoring user. Task
names belong to intents and sessions; they must not be used to create a new
agent identity for every run.

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

- an organization directory for owners and administrators, with invitations,
  roles, project permissions, session revocation, account status, and audit;
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

- passwords are hashed with Argon2id and never available to the CLI;
- PostgreSQL stores only digests of invitations, web sessions, CSRF secrets,
  device codes, and device credentials;
- server profile metadata lives outside repositories in
  `~/.config/pact/config.json` on macOS/Linux and
  `%APPDATA%\Pact\config.json` on Windows, while device credentials live in the
  operating system's credential store;
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
- native credential stores protect secrets at rest, but they do not sandbox or
  defend against hostile code already running as the same operating-system
  user;
- the one-time setup code must be removed from the server environment after
  the first owner account is created;
- production deployment requires HTTPS, backups, monitoring, and appropriate
  secret management.

Please report security issues privately as described in [SECURITY.md](SECURITY.md).

## CLI reference

| Command | Description |
|---|---|
| `pact login --server URL [--name NAME]` | Add or reauthorize a server profile through the browser device flow |
| `pact servers list [--json]` | List authorized server profiles without exposing credentials |
| `pact servers use PROFILE_OR_URL` | Select the preference for commands without a bound folder |
| `pact servers remove PROFILE_OR_URL` | Revoke and remove one server profile; use `--local-only` only for recovery |
| `pact init [--workspace UUID] [--repository UUID] [PATH]` | Create or recover a project and bind the owner checkout to its workspace and repository |
| `pact connect [--workspace UUID] [--repository UUID] [--rebind] [PATH]` | Bind another checkout to an existing destination, or explicitly replace its current binding |
| `pact status [--path PATH] [--json]` | Show the server, workspace, project, and repository bound to a checkout |
| `pact repository list` | Show the primary and additional project repositories and their verified revisions |
| `pact repository status [--repository UUID]` | Show verified state for the primary or selected repository |
| `pact repository sync [--repository UUID]` | Verify the primary or selected repository with GitHub |
| `pact enable codex` | Install the project-scoped Codex MCP configuration |
| `pact enable claude` | Install the project-scoped Claude Code MCP configuration |
| `pact invite create --email EMAIL` | Create a one-time project invitation |
| `pact join --server URL --invite-stdin` | Open an invitation registration URL in the browser |
| `pact whoami` | Show the current identity and server |
| `pact logout [--server PROFILE_OR_URL]` | Revoke and remove one server profile; the current folder wins when omitted |
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
| `PACT_SETUP_TOKEN` | One-time first-owner setup code; remove after setup |
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
| `make ui-install` | Install the locked React toolchain locally |
| `make ui-dev` | Start Vite with API proxying and hot reload |
| `make ui-build` | Build the React application for Go embedding |
| `make deploy-production` | Queue a deployment of synchronized `main` in GitHub Actions |
| `make publish-patch` | Publish the next patch release asynchronously |
| `make ps` | Show Pact containers |
| `make logs` | Follow server and database logs |
| `make test-ui` | Run the fast Admin UI validation profile |
| `make test` | Run unit tests in the reproducible Docker build |
| `make test-race` | Run the Go race detector |
| `make test-integration` | Run PostgreSQL integration tests |
| `make build` | Build the hardened server image |
| `make verify` | Validate Compose, test, build, and clean stale artifacts |
| `make docker-audit` | Show only Pact-related containers, images, volumes, and cache |
| `make docker-clean-stale` | Remove stale Pact build artifacts while preserving volumes |
| `make down` | Stop the local stack without deleting PostgreSQL data |

The control plane source lives in `web/` and uses React, TypeScript, Vite, and
Vitest. Run `make ui-install` once and `make ui-dev` alongside `make dev` for
hot reload at `http://127.0.0.1:5173/admin/`; Vite proxies the same-origin API
and health endpoints to Pact Server. `make test-ui` runs the locked TypeScript
checks, component tests, production build, and focused Go handler tests in
Docker. The generated `adminui/dist/` directory is not committed. Production
still contains one Go binary: Node is used only during the image build.

Run `make test` or `make verify` before merging changes that affect the API,
data model, infrastructure, or any other package.

Native Go checks:

```sh
make ui-build
go vet ./...
go test ./...
go test -race ./...
```

CI runs the complete suite on Windows, macOS, and Linux and cross-compiles both
Windows architectures. A release is smoke-tested by installing its published
artifact on clean runners before the workflow succeeds.

Production operations do not require keeping a local build, a GitHub CLI
session, or this terminal open. `./scripts/deploy-production.sh` pushes an
auditable deployment tag that starts the protected production workflow.
`./scripts/publish-desktop.sh patch` creates the next stable tag and
the existing release workflow builds and publishes Desktop, CLI, and PACT
Server in GitHub Actions. Both commands reject dirty, detached, or unsynchronized
working trees. Use `minor`, `major`, or an explicit `vX.Y.Z` when a patch bump is
not appropriate.

For a detailed setup and API walkthrough, see
[docs/development.md](docs/development.md). Architecture decisions live under
[docs/adr](docs/adr).

## Project layout

```text
cmd/pact/                 CLI, agent wrapper, Pact Node, and MCP adapter
cmd/pact-server/          server entry point and migrations command
internal/                 domain modules and infrastructure adapters
internal/transport/       HTTP API and embedded backoffice
web/                      React and TypeScript control plane source
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
