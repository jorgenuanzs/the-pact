package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const activeRuntimeName = "active"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "PACT MCP launcher: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve launcher path: %w", err)
	}
	root := filepath.Dir(filepath.Dir(executable))
	runtimeRoot := filepath.Join(root, "runtime")
	payload, err := os.ReadFile(filepath.Join(runtimeRoot, activeRuntimeName))
	if err != nil {
		return fmt.Errorf("read active runtime: %w", err)
	}
	digest := strings.TrimSpace(string(payload))
	if !validDigest(digest) {
		return errors.New("active runtime identifier is invalid")
	}
	name := "pact-local"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(runtimeRoot, digest, name)
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("inspect active runtime: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("active runtime is not an executable file")
	}
	if err := verifyRuntime(target, digest); err != nil {
		return err
	}

	command := exec.Command(target, os.Args[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = os.Environ()
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}
		return fmt.Errorf("start active runtime: %w", err)
	}
	return nil
}

func verifyRuntime(path, expectedPrefix string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open active runtime: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("verify active runtime: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.HasPrefix(actual, expectedPrefix) {
		return errors.New("active runtime failed its integrity check")
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 12 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
