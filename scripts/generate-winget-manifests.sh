#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 <version> <amd64-sha256> <arm64-sha256> <output-directory>" >&2
  exit 2
fi

version="${1#v}"
amd64_sha="$(printf '%s' "$2" | tr '[:lower:]' '[:upper:]')"
arm64_sha="$(printf '%s' "$3" | tr '[:lower:]' '[:upper:]')"
output_directory="$4"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid Pact version: $version" >&2
  exit 2
fi
if [[ ! "$amd64_sha" =~ ^[0-9A-F]{64}$ || ! "$arm64_sha" =~ ^[0-9A-F]{64}$ ]]; then
  echo "both installer hashes must be SHA-256 values" >&2
  exit 2
fi

mkdir -p "$output_directory"

cat >"$output_directory/Nuanzs.Pact.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.version.1.12.0.schema.json
PackageIdentifier: Nuanzs.Pact
PackageVersion: $version
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.12.0
EOF

cat >"$output_directory/Nuanzs.Pact.installer.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.installer.1.12.0.schema.json
PackageIdentifier: Nuanzs.Pact
PackageVersion: $version
InstallerType: zip
NestedInstallerType: portable
Scope: user
Commands:
  - pact
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/jorgenuanzs/the-pact/releases/download/v$version/pact_windows_amd64.zip
    InstallerSha256: $amd64_sha
    NestedInstallerFiles:
      - RelativeFilePath: pact.exe
        PortableCommandAlias: pact
  - Architecture: arm64
    InstallerUrl: https://github.com/jorgenuanzs/the-pact/releases/download/v$version/pact_windows_arm64.zip
    InstallerSha256: $arm64_sha
    NestedInstallerFiles:
      - RelativeFilePath: pact.exe
        PortableCommandAlias: pact
ManifestType: installer
ManifestVersion: 1.12.0
EOF

cat >"$output_directory/Nuanzs.Pact.locale.en-US.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.defaultLocale.1.12.0.schema.json
PackageIdentifier: Nuanzs.Pact
PackageVersion: $version
PackageLocale: en-US
Publisher: Nuanzs
PublisherUrl: https://github.com/jorgenuanzs
PackageName: Pact
PackageUrl: https://github.com/jorgenuanzs/the-pact
License: Apache-2.0
LicenseUrl: https://github.com/jorgenuanzs/the-pact/blob/v$version/LICENSE
ShortDescription: Live coordination and shared project context for people and AI agents.
Description: Pact coordinates people and AI agents around Git repositories through durable work intents, live presence, isolated worktrees, and shared operational context.
Tags:
  - ai
  - collaboration
  - developer-tools
  - git
  - mcp
ManifestType: defaultLocale
ManifestVersion: 1.12.0
EOF
