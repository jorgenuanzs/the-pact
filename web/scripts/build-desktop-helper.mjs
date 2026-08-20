import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const executableName = process.platform === "win32" ? "pact-local.exe" : "pact-local";
const outputPath = join(repositoryRoot, "desktop", "localhelper", executableName);
const version = process.env.PACT_VERSION?.trim() || "dev";
const commit = process.env.PACT_COMMIT?.trim() || "unknown";
const buildDate = process.env.PACT_BUILD_DATE?.trim() || new Date().toISOString();
const packagePath = "github.com/jorgenuanzs/the-pact/internal/buildinfo";

mkdirSync(dirname(outputPath), { recursive: true });

const result = spawnSync(
  "go",
  [
    "build",
    "-buildvcs=false",
    "-trimpath",
    `-ldflags=-s -w -X ${packagePath}.Version=${version} -X ${packagePath}.Commit=${commit} -X ${packagePath}.Date=${buildDate}`,
    "-o",
    outputPath,
    "./cmd/pact",
  ],
  { cwd: repositoryRoot, stdio: "inherit" },
);

if (result.error) {
  throw result.error;
}
if (result.status !== 0) {
  process.exit(result.status ?? 1);
}
