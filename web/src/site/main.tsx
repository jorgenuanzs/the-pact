import { StrictMode, useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";

import "./site.css";

const releasesURL = "https://api.github.com/repos/jorgenuanzs/the-pact/releases/latest";
const repositoryURL = "https://github.com/jorgenuanzs/the-pact";

type ReleaseAsset = {
  name: string;
  browser_download_url: string;
};

type Release = {
  tag_name: string;
  html_url: string;
  assets: ReleaseAsset[];
};

type Platform = "macos" | "windows" | "linux";

function detectedPlatform(): Platform {
  const value = navigator.userAgent.toLowerCase();
  if (value.includes("win")) return "windows";
  if (value.includes("linux")) return "linux";
  return "macos";
}

function asset(release: Release | null, names: string[]) {
  return release?.assets.find((candidate) => names.includes(candidate.name));
}

function BrandMark() {
  return <span className="site-brand-mark" aria-hidden="true"><i /></span>;
}

function Arrow() {
  return <span aria-hidden="true">↗</span>;
}

function ReleaseDownloads() {
  const [release, setRelease] = useState<Release | null>(null);
  const [loading, setLoading] = useState(true);
  const platform = useMemo(detectedPlatform, []);

  useEffect(() => {
    const controller = new AbortController();
    fetch(releasesURL, { signal: controller.signal, headers: { Accept: "application/vnd.github+json" } })
      .then((response) => {
        if (!response.ok) throw new Error(`GitHub returned ${response.status}`);
        return response.json() as Promise<Release>;
      })
      .then(setRelease)
      .catch(() => setRelease(null))
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, []);

  const macDesktop = asset(release, ["PACT-macOS-arm64.zip", "PACT-macOS-arm64.dmg"]);
  const windowsDesktop = asset(release, ["PACT-Windows-amd64-installer.exe", "PACT-amd64-installer.exe"]);
  const selfHostedServer = asset(release, ["pact-server-self-host.zip"]);
  const desktopChannel = asset(release, ["desktop-channel-signed.txt"])
    ? "signed"
    : asset(release, ["desktop-channel-preview.txt"])
      ? "preview"
      : undefined;
  const cli = platform === "windows"
    ? asset(release, ["pact_windows_amd64.zip"])
    : platform === "linux"
      ? asset(release, ["pact_linux_amd64.tar.gz"])
      : asset(release, ["pact_darwin_arm64.tar.gz"]);

  const preferredDesktop = platform === "windows" ? windowsDesktop : macDesktop;
  const desktopTrustMessage = loading
    ? "Checking the latest release…"
    : !preferredDesktop
      ? "Desktop preview installers are being prepared. CLI downloads are available today."
      : desktopChannel === "signed"
        ? "Signed for macOS and Windows. Updates are additionally verified with checksums and a pinned Ed25519 key."
        : desktopChannel === "preview"
          ? "Preview: this build has no Apple or Windows publisher signature yet. In-app updates are still verified with checksums and a pinned Ed25519 key."
          : "Published through the PACT release pipeline. Verify it with the release checksums.";

  return (
    <section className="site-section site-downloads" id="download">
      <div className="site-section-heading">
        <p className="site-eyebrow">Download</p>
        <h2>Start from your desktop.<br />Choose where PACT lives.</h2>
        <p>Connect to your company server now, or run the complete PACT stack locally when local server mode is enabled.</p>
      </div>

      <div className="site-download-grid">
        <article className="site-download-primary">
          <div>
            <span className="site-platform-kicker">PACT Desktop</span>
            <h3>{platform === "windows" ? "Windows" : platform === "linux" ? "macOS and Windows" : "macOS"}</h3>
            <p>The native control surface for this computer, its repositories, and its coding agents.</p>
          </div>
          {preferredDesktop ? (
            <a className="site-button site-button-lime" href={preferredDesktop.browser_download_url}>
              Download {release?.tag_name} <span>↓</span>
            </a>
          ) : (
            <a className="site-button site-button-outline" href={`${repositoryURL}/actions/workflows/desktop.yml`}>
              Desktop preview builds <Arrow />
            </a>
          )}
          <small className={desktopChannel === "preview" ? "site-download-warning" : undefined}>{desktopTrustMessage}</small>
        </article>

        <article className="site-download-card">
          <span className="site-platform-kicker">Command line</span>
          <h3>PACT CLI</h3>
          <p>Initialize projects, connect agents through MCP, and operate PACT from a terminal.</p>
          {cli ? <a href={cli.browser_download_url}>Download {release?.tag_name} <Arrow /></a> : <a href={`${repositoryURL}/releases/latest`}>View latest release <Arrow /></a>}
        </article>

        <article className="site-download-card">
          <span className="site-platform-kicker">Infrastructure</span>
          <h3>PACT Server</h3>
          <p>Host the shared source of truth on your computer, a VM, or your own cloud account.</p>
          {selfHostedServer ? (
            <a href={selfHostedServer.browser_download_url}>Download {release?.tag_name} <Arrow /></a>
          ) : (
            <a href="#self-host">Self-hosting options <span>↓</span></a>
          )}
        </article>
      </div>
    </section>
  );
}

function Site() {
  return (
    <div className="site-shell">
      <header className="site-nav">
        <a className="site-brand" href="#top"><BrandMark /><strong>PACT</strong></a>
        <nav aria-label="Main navigation">
          <a href="#product">Product</a>
          <a href="#modes">Modes</a>
          <a href="#download">Download</a>
          <a href={repositoryURL}>GitHub</a>
        </nav>
        <a className="site-nav-access" href="/admin/">Open PACT <Arrow /></a>
      </header>

      <main id="top">
        <section className="site-hero">
          <div className="site-hero-copy">
            <p className="site-eyebrow"><span /> Live project coordination</p>
            <h1>Your project stays alive while people and agents work.</h1>
            <p className="site-hero-lede">PACT gives every contributor a shared, current understanding of the code, decisions, conversations, and work already in motion.</p>
            <div className="site-actions">
              <a className="site-button site-button-lime" href="#download">Download PACT <span>↓</span></a>
              <a className="site-button site-button-ghost" href={repositoryURL}>View source <Arrow /></a>
            </div>
          </div>

          <div className="site-live-card" aria-label="Example of live PACT coordination">
            <div className="site-live-header"><span><i /> Footfall</span><small>LIVE · 5 ACTORS</small></div>
            <div className="site-live-alert"><b>COLLISION</b><span>Two intents claim the same repository scope.</span></div>
            <div className="site-actors">
              <span><i className="lime">CX</i> Codex <b>working</b></span>
              <span><i>CL</i> Claude <em>blocked</em></span>
              <span><i className="dark">J</i> Jorge <b>reviewing</b></span>
            </div>
            <div className="site-work-table">
              <div className="head"><span>ACTOR</span><span>OBJECTIVE</span><span>STATUS</span></div>
              <div><strong>Codex</strong><span>Implement traffic filters</span><b>Active</b></div>
              <div><strong>Claude</strong><span>Adjust zone aggregation</span><em>Blocked</em></div>
              <div><strong>Jorge</strong><span>Review analytics contract</span><b>Active</b></div>
            </div>
            <div className="site-live-footer"><span><i /> Context accepted</span><code>main · a81c2e0</code></div>
          </div>
        </section>

        <section className="site-principle" id="product">
          <p className="site-eyebrow">A shared operational memory</p>
          <p className="site-statement">Git stores what changed. PACT keeps track of <strong>what is changing now, why it matters, and who is responsible.</strong></p>
          <div className="site-capability-grid">
            <article><span>01</span><h3>Live work</h3><p>See active intents, reserved scopes, collisions, heartbeats, and handoffs before changes overlap.</p></article>
            <article><span>02</span><h3>Shared context</h3><p>Turn decisions, risks, questions, documents, and conversations into durable project knowledge.</p></article>
            <article><span>03</span><h3>Repository awareness</h3><p>Connect every repository that belongs to the workspace and observe its canonical Git state.</p></article>
            <article><span>04</span><h3>Human ownership</h3><p>Agents operate with explicit identities, access, responsibilities, and a complete activity trail.</p></article>
          </div>
        </section>

        <section className="site-modes" id="modes">
          <div className="site-section-heading">
            <p className="site-eyebrow">One architecture, three modes</p>
            <h2>Start alone.<br />Grow into a team.</h2>
          </div>
          <div className="site-mode-list">
            <article><span>01</span><div><h3>Connect to a PACT Server</h3><p>Install Desktop and authorize this computer against the server your company already operates.</p></div><small>REMOTE</small></article>
            <article><span>02</span><div><h3>Run PACT locally</h3><p>Desktop manages PACT Server, PostgreSQL, pgvector, upgrades, and backups on this computer.</p></div><small>PERSONAL</small></article>
            <article><span>03</span><div><h3>Self-host for your organization</h3><p>Deploy the same server on a VM or cloud environment and connect every person and agent to it.</p></div><small>TEAM</small></article>
          </div>
        </section>

        <ReleaseDownloads />

        <section className="site-self-host" id="self-host">
          <div>
            <p className="site-eyebrow">Your infrastructure, your data</p>
            <h2>PACT Server is designed to live wherever your project does.</h2>
            <p>The server is the shared source of truth. Desktop and CLI are clients. Local and remote installations use the same API, database model, and migration path.</p>
          </div>
          <pre><code><span>$</span> pact server install{"\n"}<span>$</span> pact server start{"\n"}<span>✓</span> PACT is ready at 127.0.0.1:8080</code></pre>
        </section>

        <section className="site-final">
          <BrandMark />
          <h2>Give every contributor the same project.</h2>
          <div className="site-actions">
            <a className="site-button site-button-lime" href="#download">Get PACT <span>↓</span></a>
            <a className="site-button site-button-dark" href="/admin/">Open the control plane <Arrow /></a>
          </div>
        </section>
      </main>

      <footer className="site-footer">
        <a className="site-brand" href="#top"><BrandMark /><strong>PACT</strong></a>
        <p>Live coordination and shared project context for people and AI coding agents.</p>
        <div><a href={repositoryURL}>GitHub</a><a href={`${repositoryURL}/blob/main/README.md`}>Documentation</a><a href={`${repositoryURL}/blob/main/SECURITY.md`}>Security</a></div>
      </footer>
    </div>
  );
}

createRoot(document.getElementById("site-root")!).render(<StrictMode><Site /></StrictMode>);
