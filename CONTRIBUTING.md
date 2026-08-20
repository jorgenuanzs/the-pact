# Contributing to Pact

Thank you for helping build Pact. The project is in early alpha, so focused
issues, design feedback, tests, and small coherent pull requests are especially
valuable.

## Before opening a change

- Read the [README](README.md) and the relevant records under
  [docs/adr](docs/adr).
- Search existing issues and pull requests before creating a duplicate.
- Open a design discussion before changing a protocol, persistence invariant,
  authentication boundary, or public API.
- Never include credentials, private infrastructure details, customer data, or
  AI conversation contents in an issue, fixture, log, or commit.

## Design principles

Contributions should preserve these project rules:

- Git remains the authority for repository files and history.
- Pact coordinates intent and live state around Git; it does not invent a
  proprietary source repository.
- Identity, authorization, leases, events, and mutations are deterministic and
  auditable.
- AI-generated summaries or inferences are derived data, not authority.
- Secrets are referenced or mediated, never placed in prompts or project files.
- Missing authorization denies access by default.
- Local and server-side data boundaries must remain explicit.

## Development setup

The reproducible path uses Docker:

```sh
make init
# Set PACT_DB_PASSWORD and PACT_SETUP_TOKEN in .env for first-time setup.
make doctor
make dev
```

Run the standard verification suite:

```sh
make test
make test-race
make test-integration
make build
```

If the Go toolchain declared in `go.mod` is installed locally, build the
embedded React application first:

```sh
make ui-install
make ui-build
go vet ./...
go test ./...
go test -race ./...
```

Do not commit `.env`, `.pact/`, Terraform state, build output, database dumps,
or local MCP configuration.

## Pull requests

A pull request should:

- explain the problem and the chosen behavior;
- keep unrelated refactors separate;
- include tests for observable behavior and security boundaries;
- update `api/openapi.yaml` when the HTTP contract changes;
- add or update an ADR for a durable architectural decision;
- update user-facing documentation and examples;
- pass CI on Windows, macOS, and Linux;
- avoid generated, vendored, or binary files unless the change specifically
  requires them.

Use clear imperative commit messages. Conventional prefixes such as `feat:`,
`fix:`, `docs:`, `test:`, and `chore:` are encouraged but not required.

## Database migrations

Migrations under `internal/platform/migrations/sql` are immutable after release
and protected by checksums. Add a new numbered migration instead of editing a
released one. State changes, durable events, and outbox writes must remain in
the same transaction.

## Security changes

Security fixes should not begin in a public issue. Follow
[SECURITY.md](SECURITY.md) so the report and remediation can remain private
until a release is available.

## License

By intentionally submitting a contribution for inclusion in Pact, you agree
that it is provided under the project's [Apache License 2.0](LICENSE), as
described in Section 5 of that license.
