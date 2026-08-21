# Changelog

This file records user-visible changes included in each PACT release.

Add new work to **Unreleased** as it is completed. When publishing a version,
move those entries into a versioned section with the release date and use them
as the basis for the GitHub release notes.

## Unreleased

### Added

- Added the shared multi-server profile registry foundation for Desktop, CLI,
  and the local runtime, including stable profile IDs and per-server identity
  metadata.
- Added `pact servers list`, `pact servers use`, `pact servers remove`,
  server-specific logout, named profiles, and `pact status` for inspecting a
  checkout's resolved server, workspace, project, and repository.
- Added folder binding schema v2 with explicit workspace and repository IDs,
  a normalized Git remote fingerprint, and binding timestamps.
- Added `--workspace` and `--repository` disambiguation to `pact init` and
  `pact connect`, plus explicit, idempotent `pact connect --rebind` support.

### Changed

- CLI, MCP, and runtime commands now resolve the credential named by the
  checkout binding instead of comparing it with the active profile. The active
  profile is only a preference for commands that have no bound folder.
- Legacy folder bindings are enriched only after the server has validated the
  visible workspace and repository. An offline server leaves the v1 binding
  untouched and migration resumes on a later command.
- Local node identities now record their server. They remain stable during a
  same-server schema upgrade and rotate when a checkout moves to another
  server.

### Security

- Moved device credentials out of `config.json` into macOS Keychain, Windows
  Credential Manager, or the platform-native user keyring. Existing v2
  configurations migrate atomically and remain untouched if secure storage
  cannot be verified.
- Added atomic cross-platform registry writes and an inter-process lock so
  Desktop and CLI cannot overwrite concurrent profile updates.
- Folder bindings are written atomically under an inter-process lock and store
  only a SHA-256 fingerprint of the normalized Git remote, never its embedded
  credentials or raw URL.

### Fixed

- Added clearer spacing between the create-workspace button and the local
  controls in the global sidebar.
