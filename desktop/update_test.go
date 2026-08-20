package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
