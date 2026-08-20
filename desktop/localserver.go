package main

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/localserver"
)

type LocalServerInstallInput struct {
	Port  int    `json:"port"`
	Image string `json:"image,omitempty"`
}

type LocalServerInstallResult struct {
	Status    localserver.Status `json:"status"`
	SetupCode string             `json:"setup_code"`
}

type LocalServerUpgradeResult struct {
	Status localserver.Status `json:"status"`
	Backup string             `json:"backup"`
}

func desktopLocalServerManager() (*localserver.Manager, error) {
	return localserver.NewDefault(io.Discard, io.Discard)
}

func (d *Desktop) LocalServerStatus() (localserver.Status, error) {
	manager, err := desktopLocalServerManager()
	if err != nil {
		return localserver.Status{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return manager.Status(ctx)
}

func (d *Desktop) InstallLocalServer(input LocalServerInstallInput) (LocalServerInstallResult, error) {
	manager, err := desktopLocalServerManager()
	if err != nil {
		return LocalServerInstallResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	result, err := manager.Install(ctx, localserver.InstallOptions{Port: input.Port, Image: strings.TrimSpace(input.Image)})
	if err != nil {
		return LocalServerInstallResult{}, err
	}
	return LocalServerInstallResult{Status: result.Status, SetupCode: result.SetupCode}, nil
}

func (d *Desktop) StartLocalServer() (localserver.Status, error) {
	manager, err := desktopLocalServerManager()
	if err != nil {
		return localserver.Status{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return manager.Start(ctx)
}

func (d *Desktop) StopLocalServer() (localserver.Status, error) {
	manager, err := desktopLocalServerManager()
	if err != nil {
		return localserver.Status{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return manager.Stop(ctx)
}

func (d *Desktop) BackupLocalServer() (string, error) {
	manager, err := desktopLocalServerManager()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return manager.Backup(ctx, "")
}

func (d *Desktop) UpgradeLocalServer(image string) (LocalServerUpgradeResult, error) {
	manager, err := desktopLocalServerManager()
	if err != nil {
		return LocalServerUpgradeResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	status, backup, err := manager.Upgrade(ctx, strings.TrimSpace(image))
	if err != nil {
		return LocalServerUpgradeResult{}, err
	}
	return LocalServerUpgradeResult{Status: status, Backup: backup}, nil
}
