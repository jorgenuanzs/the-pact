import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const requested = (process.argv[2] || "").trim();
const match = requested.match(/^v?(\d+)\.(\d+)\.(\d+)/);
if (!match) {
  throw new Error(`Expected a semantic release version, received ${JSON.stringify(requested)}`);
}

const version = `${match[1]}.${match[2]}.${match[3]}`;
const path = resolve("desktop/build/config.yml");
const config = readFileSync(path, "utf8");
const versionLine = /^(\s*version:\s*)"[^"]+"(\s*#.*)?$/m;
if (!versionLine.test(config)) {
  throw new Error(`Could not update info.version in ${path}`);
}
const updated = config.replace(versionLine, `$1"${version}"$2`);
writeFileSync(path, updated);
console.log(`PACT Desktop product version: ${version}`);
