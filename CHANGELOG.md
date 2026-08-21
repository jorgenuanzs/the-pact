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

### Security

- Moved device credentials out of `config.json` into macOS Keychain, Windows
  Credential Manager, or the platform-native user keyring. Existing v2
  configurations migrate atomically and remain untouched if secure storage
  cannot be verified.
- Added atomic cross-platform registry writes and an inter-process lock so
  Desktop and CLI cannot overwrite concurrent profile updates.

### Fixed

- Added clearer spacing between the create-workspace button and the local
  controls in the global sidebar.
