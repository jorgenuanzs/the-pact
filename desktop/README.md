# PACT Desktop

PACT Desktop is the native macOS and Windows client for a PACT Server. It uses
the same React control plane as the web application and adds a small Go bridge
for operating-system integration.

## What Desktop supports

PACT Desktop supports remote and local PACT Servers:

- enter a server URL;
- authorize the computer through the server's existing device flow;
- reuse the authenticated control plane without exposing the device credential
  to JavaScript;
- proxy versioned API calls through the native Go layer;
- receive project events through a native reconnecting SSE client;
- revoke or forget the desktop connection;
- manage device-specific state from a separate **This computer** area;
- detect Codex and Claude Code, select a checkout with the native folder
  picker, and install their project-scoped MCP configuration;
- bundle and extract a native `pact-local` runtime so MCP keeps working when
  the PACT Desktop window or the standalone CLI is closed.
- install and operate a private local PACT Server with PostgreSQL + pgvector
  through Docker Compose;
- start, stop, back up, and upgrade that local server without requiring a
  separate CLI installation;
- check for Desktop updates in the application, verify both SHA-256 and a
  pinned Ed25519 signature, install atomically, and restart into the new
  version.

The CLI, the bundled local runtime, and Desktop share the per-user PACT device
configuration. No credential is written into a project checkout. Desktop keeps
only absolute local paths and client bindings in its own device-local registry.
Codex and Claude launch the bundled runtime on demand; it is not coupled to the
GUI process.

Desktop keeps server administration separate from this computer's agent and
checkout configuration. Remote mode connects this computer to a team server;
local mode installs the same versioned server image and database stack on the
computer itself.

## Develop

Requirements:

- Go as declared in `go.mod`;
- Node.js and npm;
- the platform requirements from the Wails documentation;
- Wails v3.0.0-beta.11 (pinned by the build and release workflows).

From the repository root:

```sh
make desktop-install
make desktop-test
make desktop-dev
```

`make desktop-dev` starts Vite and the native Wails shell with live reload.
The frontend build first compiles the current platform's CLI into an embedded
helper. This generated binary is ignored by Git and verified by native tests.

## Package

Build the package for the current operating system:

```sh
make desktop-build
```

On macOS this creates `desktop/build/bin/PACT.app`. On Windows, an NSIS
installer is built by CI with per-user installation scope. Desktop packages
must be built on their target operating system because Wails uses the native
WebView and packaging toolchain.

The first updater-capable package must still be installed manually. Every
subsequent stable package can be installed from **This computer → Local
runtime → Desktop updates**.

## Release trust and signing

PACT uses two independent signature layers:

1. the operating system trusts the application through Apple Developer ID +
   notarization on macOS and Authenticode on Windows;
2. PACT Desktop trusts update bytes through a pinned Ed25519 public key. The
   release publishes a detached `.sig` for each native update archive and the
   application requires both that signature and `checksums.txt`.

Stable Desktop releases fail closed unless every platform credential exists.
GitHub Actions expects these repository secrets:

- `MACOS_CERTIFICATE`, `MACOS_CERTIFICATE_PASSWORD`,
  `MACOS_SIGNING_IDENTITY`, `APPLE_ID`, `APPLE_APP_PASSWORD`, `APPLE_TEAM_ID`;
- `WINDOWS_CERTIFICATE`, `WINDOWS_CERTIFICATE_PASSWORD`;
- `PACT_UPDATER_PRIVATE_KEY`, containing the base64-encoded PKCS#8 Ed25519
  private key whose public half is pinned in `update.go`.

The private keys never enter the repository. Preview tags may omit the
operating-system certificates, but update archives always require the Ed25519
release signature. The stable updater ignores prereleases.
