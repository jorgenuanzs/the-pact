import { describe, expect, it } from "vitest";

import { hasDesktopRuntime, isWailsAssetURL } from "./desktop";

describe("isWailsAssetURL", () => {
  it.each([
    "wails://wails/",
    "wails://localhost/index.html",
    "https://wails.localhost/",
  ])("recognizes a packaged Wails URL: %s", (address) => {
    expect(isWailsAssetURL(address)).toBe(true);
  });

  it.each([
    "https://pact.nuanzs.com/admin/",
    "http://localhost:5173/admin/",
    "not a URL",
  ])("does not treat a regular web URL as Wails: %s", (address) => {
    expect(isWailsAssetURL(address)).toBe(false);
  });
});

describe("hasDesktopRuntime", () => {
  it("detects Wails before its injected environment is available", () => {
    expect(hasDesktopRuntime("wails://wails/")).toBe(true);
    expect(hasDesktopRuntime("https://wails.localhost/")).toBe(true);
  });

  it("keeps a regular web session in browser mode", () => {
    expect(hasDesktopRuntime("https://pact.nuanzs.com/admin/")).toBe(false);
  });
});
