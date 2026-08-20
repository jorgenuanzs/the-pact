import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";

const directory = resolve(process.argv[2] || "dist");
const output = join(directory, "release-manifest.json");
const excluded = new Set(["checksums.txt", "release-manifest.json"]);

function files(root) {
  return readdirSync(root, { withFileTypes: true })
    .flatMap((entry) => entry.isDirectory() ? files(join(root, entry.name)) : [join(root, entry.name)]);
}

const assets = files(directory)
  .filter((path) => !excluded.has(relative(directory, path)))
  .sort()
  .map((path) => {
    const body = readFileSync(path);
    return {
      name: relative(directory, path).replaceAll("\\", "/"),
      bytes: statSync(path).size,
      sha256: createHash("sha256").update(body).digest("hex"),
    };
  });

const manifest = {
  schema_version: 1,
  version: process.env.PACT_VERSION || "dev",
  commit: process.env.PACT_COMMIT || "unknown",
  created_at: process.env.PACT_BUILD_DATE || new Date().toISOString(),
  server_image: `ghcr.io/jorgenuanzs/the-pact:${process.env.PACT_VERSION || "edge"}`,
  desktop_channel: process.env.PACT_DESKTOP_CHANNEL || "preview",
  assets,
};

writeFileSync(output, `${JSON.stringify(manifest, null, 2)}\n`);
