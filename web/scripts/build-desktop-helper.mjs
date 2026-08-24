import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const executableSuffix = process.platform === "win32" ? ".exe" : "";
const helperDirectory = join(repositoryRoot, "desktop", "localhelper");
const version = process.env.PACT_VERSION?.trim() || "dev";
const commit = process.env.PACT_COMMIT?.trim() || "unknown";
const buildDate = process.env.PACT_BUILD_DATE?.trim() || new Date().toISOString();
const packagePath = "github.com/jorgenuanzs/the-pact/internal/buildinfo";

mkdirSync(helperDirectory, { recursive: true });

const helpers = [
  {
    name: `pact-local${executableSuffix}`,
    package: "./cmd/pact",
    ldflags: `-s -w -X ${packagePath}.Version=${version} -X ${packagePath}.Commit=${commit} -X ${packagePath}.Date=${buildDate}`,
  },
  {
    name: `pact-mcp-launcher${executableSuffix}`,
    package: "./cmd/pact-mcp-launcher",
    ldflags: "-s -w",
  },
];

for (const helper of helpers) {
  const outputPath = join(helperDirectory, helper.name);
  mkdirSync(dirname(outputPath), { recursive: true });
  const result = spawnSync(
    "go",
    [
      "build",
      "-buildvcs=false",
      "-trimpath",
      `-ldflags=${helper.ldflags}`,
      "-o",
      outputPath,
      helper.package,
    ],
    { cwd: repositoryRoot, stdio: "inherit" },
  );
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}
