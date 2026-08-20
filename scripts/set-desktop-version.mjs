import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const requested = (process.argv[2] || "").trim();
const match = requested.match(/^v?(\d+)\.(\d+)\.(\d+)/);
if (!match) {
  throw new Error(`Expected a semantic release version, received ${JSON.stringify(requested)}`);
}

const version = `${match[1]}.${match[2]}.${match[3]}`;
const path = resolve("desktop/wails.json");
const config = JSON.parse(readFileSync(path, "utf8"));
config.info = { ...config.info, productVersion: version };
writeFileSync(path, `${JSON.stringify(config, null, 2)}\n`);
console.log(`PACT Desktop product version: ${version}`);
