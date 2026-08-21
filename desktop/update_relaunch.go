package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const updateRelaunchDelay = 2 * time.Second

// markUpdateRelaunch records that the next macOS launch will be started by
// Wails' updater helper. That helper briefly overlaps with the new process
// while it cleans the replaced bundle; creating WKWebView during that window
// can leave the first relaunched window blank.
func markUpdateRelaunch() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	markerPath, err := updateRelaunchMarkerPath()
	if err != nil {
		return err
	}
	return writeUpdateRelaunchMarker(markerPath, currentVersion)
}

// waitForUpdateRelaunch consumes the one-shot marker before Wails or WKWebView
// are initialised. The helper has exited by the end of this short pause, so the
// new application gets a clean first launch instead of requiring a manual
// close and reopen.
func waitForUpdateRelaunch() {
	if runtime.GOOS != "darwin" {
		return
	}
	markerPath, err := updateRelaunchMarkerPath()
	if err != nil {
		return
	}
	pauseForUpdateRelaunch(markerPath, time.Sleep)
}

func updateRelaunchMarkerPath() (string, error) {
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(cacheDirectory, "com.nuanzs.pact", "update-relaunch.pending"), nil
}

func writeUpdateRelaunchMarker(markerPath, version string) (resultErr error) {
	directory := filepath.Dir(markerPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create update relaunch directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, "update-relaunch-*.tmp")
	if err != nil {
		return fmt.Errorf("create update relaunch marker: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if resultErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure update relaunch marker: %w", err)
	}
	if _, err := fmt.Fprintln(temporary, version); err != nil {
		return fmt.Errorf("write update relaunch marker: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close update relaunch marker: %w", err)
	}
	if err := os.Rename(temporaryPath, markerPath); err != nil {
		return fmt.Errorf("activate update relaunch marker: %w", err)
	}
	return nil
}

func consumeUpdateRelaunchMarker(markerPath string) bool {
	if err := os.Remove(markerPath); err != nil {
		return false
	}
	return true
}

func pauseForUpdateRelaunch(markerPath string, pause func(time.Duration)) bool {
	if !consumeUpdateRelaunchMarker(markerPath) {
		return false
	}
	pause(updateRelaunchDelay)
	return true
}
