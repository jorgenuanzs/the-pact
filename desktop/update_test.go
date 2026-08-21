package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	githubupdates "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestDesktopUpdateAssetMatcherUsesExactPlatformAsset(t *testing.T) {
	assets := []githubupdates.ReleaseAsset{
		{Name: "PACT-macOS-arm64.zip"},
		{Name: "PACT-darwin-arm64.zip"},
		{Name: "PACT-windows-amd64.zip"},
	}
	if got := desktopUpdateAssetMatcher(updater.CheckRequest{Platform: "darwin", Arch: "arm64"}, assets); got != 1 {
		t.Fatalf("darwin matcher = %d, want 1", got)
	}
	if got := desktopUpdateAssetMatcher(updater.CheckRequest{Platform: "windows", Arch: "amd64"}, assets); got != 2 {
		t.Fatalf("windows matcher = %d, want 2", got)
	}
	if got := desktopUpdateAssetMatcher(updater.CheckRequest{Platform: "linux", Arch: "amd64"}, assets); got != -1 {
		t.Fatalf("linux matcher = %d, want -1", got)
	}
}

func TestSignedGitHubProviderLoadsDetachedSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("artifact"))
	signature := ed25519.Sign(privateKey, digest[:])
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(base64.StdEncoding.EncodeToString(signature))),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	provider := &signedGitHubProvider{client: client}
	loaded, err := provider.fetchSignature(context.Background(), "https://updates.example/PACT.zip.sig")
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(publicKey, digest[:], loaded) {
		t.Fatal("downloaded signature did not verify")
	}
}

func TestUpdateRelaunchMarkerIsAtomicAndOneShot(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "nested", "update-relaunch.pending")
	versionPath := filepath.Join(filepath.Dir(markerPath), "last-launched-version")
	if err := writeUpdateRelaunchState(markerPath, "0.16.2"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(contents)); got != "0.16.2" {
		t.Fatalf("marker version = %q, want 0.16.2", got)
	}
	var waited time.Duration
	if !pauseForUpdateRelaunch(markerPath, versionPath, "0.16.3", func(duration time.Duration) { waited = duration }) {
		t.Fatal("first marked relaunch was not detected")
	}
	if waited != updateRelaunchDelay {
		t.Fatalf("relaunch delay = %s, want %s", waited, updateRelaunchDelay)
	}
	if pauseForUpdateRelaunch(markerPath, versionPath, "0.16.3", func(time.Duration) { t.Fatal("second launch must not pause") }) {
		t.Fatal("marker must only be consumed once")
	}
}

func TestUpdateRelaunchBootstrapsFromVersionTransition(t *testing.T) {
	directory := t.TempDir()
	markerPath := filepath.Join(directory, "update-relaunch.pending")
	versionPath := filepath.Join(directory, "last-launched-version")

	var waits int
	if !pauseForUpdateRelaunch(markerPath, versionPath, "0.16.5", func(time.Duration) { waits++ }) {
		t.Fatal("first launch of a release version must pause without a legacy marker")
	}
	if pauseForUpdateRelaunch(markerPath, versionPath, "0.16.5", func(time.Duration) { waits++ }) {
		t.Fatal("second launch of the same version must not pause")
	}
	if !pauseForUpdateRelaunch(markerPath, versionPath, "0.16.6", func(time.Duration) { waits++ }) {
		t.Fatal("first launch of the next version must pause")
	}
	if waits != 2 {
		t.Fatalf("relaunch pauses = %d, want 2", waits)
	}

	contents, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(contents)); got != "0.16.6" {
		t.Fatalf("last launched version = %q, want 0.16.6", got)
	}
}

func TestUpdateRelaunchDoesNotDelayDevelopmentBuilds(t *testing.T) {
	directory := t.TempDir()
	if pauseForUpdateRelaunch(
		filepath.Join(directory, "update-relaunch.pending"),
		filepath.Join(directory, "last-launched-version"),
		"dev",
		func(time.Duration) { t.Fatal("development launch must not pause") },
	) {
		t.Fatal("development launch was treated as an update")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
