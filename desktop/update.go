package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	githubupdates "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const (
	updateRepository = "jorgenuanzs/the-pact"
	// This public key is intentionally embedded in every PACT Desktop build.
	// The matching private key must live only in GitHub Actions and signs the
	// SHA-256 digest of every native update artifact.
	updatePublicKeyHex = "0924820f25c7979aa0be523a6f02ac5a4346e220ad3dc3476e191f8de42329cb"
)

var (
	currentVersion        = "dev"
	currentCommit         = "unknown"
	releaseVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
)

type DesktopUpdateStatus struct {
	Configured     bool   `json:"configured"`
	CurrentVersion string `json:"current_version"`
	Commit         string `json:"commit"`
	State          string `json:"state"`
	Error          string `json:"error,omitempty"`
}

type signedGitHubProvider struct {
	provider *githubupdates.Provider
	client   *http.Client
}

func (p *signedGitHubProvider) Name() string { return "github-signed" }

func (p *signedGitHubProvider) Check(ctx context.Context, request updater.CheckRequest) (*updater.Release, error) {
	release, err := p.provider.Check(ctx, request)
	if err != nil || release == nil {
		return release, err
	}
	if release.Verification == nil || release.Verification.DigestAlgo != "sha256" || len(release.Verification.Digest) == 0 {
		return nil, errors.New("release does not contain the required SHA-256 checksum")
	}
	assetURL, ok := release.Metadata["github.asset.url"].(string)
	if !ok || strings.TrimSpace(assetURL) == "" {
		return nil, errors.New("release does not contain an artifact URL")
	}
	signature, err := p.fetchSignature(ctx, assetURL+".sig")
	if err != nil {
		return nil, err
	}
	release.Verification.SignatureAlgo = "ed25519"
	release.Verification.Signature = signature
	return release, nil
}

func (p *signedGitHubProvider) Download(ctx context.Context, release *updater.Release, destination io.Writer, progress func(written, total int64)) error {
	return p.provider.Download(ctx, release, destination, progress)
}

func (p *signedGitHubProvider) fetchSignature(ctx context.Context, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("create signature request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download update signature: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("download update signature: HTTP %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return nil, fmt.Errorf("read update signature: %w", err)
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return nil, fmt.Errorf("decode update signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid update signature length: got %d bytes", len(signature))
	}
	return signature, nil
}

func (d *Desktop) configureUpdater() error {
	if !releaseVersionPattern.MatchString(currentVersion) {
		return fmt.Errorf("updates are disabled for development version %q", currentVersion)
	}
	app := d.application()
	if app == nil {
		return errors.New("desktop application is not ready")
	}
	publicKey, err := hex.DecodeString(updatePublicKeyHex)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("embedded update public key is invalid")
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	provider, err := githubupdates.New(githubupdates.Config{
		Repository:    updateRepository,
		ChecksumAsset: "checksums.txt",
		HTTPClient:    httpClient,
		AssetMatcher:  desktopUpdateAssetMatcher,
	})
	if err != nil {
		return fmt.Errorf("configure GitHub updater: %w", err)
	}
	return app.Updater.Init(updater.Config{
		CurrentVersion: currentVersion,
		Providers: []updater.Provider{
			&signedGitHubProvider{provider: provider, client: httpClient},
		},
		PublicKey: publicKey,
	})
}

func desktopUpdateAssetMatcher(request updater.CheckRequest, assets []githubupdates.ReleaseAsset) int {
	wanted := ""
	switch request.Platform + "/" + request.Arch {
	case "darwin/arm64":
		wanted = "PACT-darwin-arm64.zip"
	case "windows/amd64":
		wanted = "PACT-windows-amd64.zip"
	default:
		return -1
	}
	for index, asset := range assets {
		if asset.Name == wanted {
			return index
		}
	}
	return -1
}

func (d *Desktop) UpdateStatus() DesktopUpdateStatus {
	result := DesktopUpdateStatus{
		CurrentVersion: currentVersion,
		Commit:         currentCommit,
		State:          string(updater.StateUnconfigured),
	}
	d.mu.Lock()
	app := d.app
	result.Error = d.updateError
	d.mu.Unlock()
	if app == nil || !releaseVersionPattern.MatchString(currentVersion) || result.Error != "" {
		return result
	}
	result.Configured = true
	result.State = string(app.Updater.State())
	return result
}

func (d *Desktop) CheckForUpdates() (DesktopUpdateStatus, error) {
	status := d.UpdateStatus()
	if !status.Configured {
		if status.Error == "" {
			status.Error = "automatic updates are not configured in this build"
		}
		return status, errors.New(status.Error)
	}
	app := d.application()
	if app == nil {
		return status, errors.New("desktop application is not ready")
	}
	if err := app.Updater.CheckAndInstall(context.Background()); err != nil {
		return d.UpdateStatus(), err
	}
	return d.UpdateStatus(), nil
}
