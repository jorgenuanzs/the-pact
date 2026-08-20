import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

// The real Wails runtime keeps short-lived browser polling timers alive while it
// waits for the native host. Unit tests run without that host, so those timers
// can outlive jsdom and touch `window` after teardown. Tests that exercise the
// desktop bridge provide the legacy-compatible window mock explicitly.
vi.mock("@wailsio/runtime", () => ({
  Call: {
    ByID: vi.fn(() => Promise.resolve(undefined)),
  },
  Create: vi.fn((value) => value),
  Events: {
    On: vi.fn(() => () => undefined),
  },
  System: {
    IsDesktop: vi.fn(() => false),
  },
}));
