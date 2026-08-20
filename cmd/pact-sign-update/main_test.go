package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignArtifactProducesVerifiableDigestSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parsePrivateKey(base64.StdEncoding.EncodeToString(der))
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "PACT.zip")
	body := []byte("signed PACT update")
	if err := os.WriteFile(artifact, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := signArtifact(artifact, parsed); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(artifact + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	if !ed25519.Verify(publicKey, digest[:], signature) {
		t.Fatal("signature did not verify")
	}
}
