// Command pact-sign-update creates the detached Ed25519 signature consumed by
// PACT Desktop. It is intended for GitHub Actions, not for end users.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const privateKeyEnvironment = "PACT_UPDATER_PRIVATE_KEY"

func main() {
	file := flag.String("file", "", "artifact to sign")
	flag.Parse()
	if strings.TrimSpace(*file) == "" {
		fatal(errors.New("-file is required"))
	}
	key, err := parsePrivateKey(os.Getenv(privateKeyEnvironment))
	if err != nil {
		fatal(err)
	}
	if err := signArtifact(*file, key); err != nil {
		fatal(err)
	}
}

func parsePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("%s is required", privateKeyEnvironment)
	}
	der, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", privateKeyEnvironment, err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", privateKeyEnvironment, err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an Ed25519 private key", privateKeyEnvironment)
	}
	return key, nil
}

func signArtifact(path string, key ed25519.PrivateKey) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	signature := ed25519.Sign(key, hash.Sum(nil))
	encoded := base64.StdEncoding.EncodeToString(signature) + "\n"
	if err := os.WriteFile(path+".sig", []byte(encoded), 0o644); err != nil {
		return fmt.Errorf("write signature: %w", err)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
