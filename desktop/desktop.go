package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/authn"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/userconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type Desktop struct {
	mu          sync.Mutex
	app         *application.App
	streams     map[string]context.CancelFunc
	updateError string
}

type DesktopStatus struct {
	Configured bool              `json:"configured"`
	Connected  bool              `json:"connected"`
	ServerURL  string            `json:"server_url,omitempty"`
	Principal  *access.Principal `json:"principal,omitempty"`
	Error      string            `json:"error,omitempty"`
	DefaultURL string            `json:"default_url"`
}

type DeviceLogin struct {
	ServerURL       string `json:"server_url"`
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresAt       string `json:"expires_at"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type DeviceLoginResult struct {
	Status    string            `json:"status"`
	Connected bool              `json:"connected"`
	Principal *access.Principal `json:"principal,omitempty"`
	ExpiresAt string            `json:"expires_at,omitempty"`
}

func NewDesktop() *Desktop {
	return &Desktop{streams: make(map[string]context.CancelFunc)}
}

func (d *Desktop) attachApplication(app *application.App) {
	d.mu.Lock()
	d.app = app
	d.mu.Unlock()
	if err := d.configureUpdater(); err != nil {
		d.mu.Lock()
		d.updateError = err.Error()
		d.mu.Unlock()
	}
}

func (d *Desktop) ServiceShutdown() error {
	d.stopAllStreams()
	return nil
}

func (d *Desktop) Status() DesktopStatus {
	result := DesktopStatus{}
	config, err := userconfig.Load()
	if err != nil {
		if !strings.Contains(err.Error(), "not logged in") {
			result.Error = err.Error()
		}
		return result
	}
	result.Configured = true
	result.ServerURL = config.ServerURL

	client, err := pactclient.New(config.ServerURL, config.DeviceCredential)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	principal, err := client.Me(ctx)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Connected = true
	result.Principal = &principal
	return result
}

func (d *Desktop) BeginDeviceLogin(serverURL string) (DeviceLogin, error) {
	normalized, err := userconfig.NormalizeServerURL(serverURL)
	if err != nil {
		return DeviceLogin{}, err
	}
	name := desktopDeviceName()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	authorization, err := pactclient.BeginDeviceAuthorization(ctx, normalized, authn.BeginDeviceInput{DeviceName: name})
	if err != nil {
		return DeviceLogin{}, fmt.Errorf("begin device authorization: %w", err)
	}
	verificationURL, err := resolveURL(normalized, authorization.VerificationURI)
	if err != nil {
		return DeviceLogin{}, err
	}
	if app := d.application(); app != nil {
		_ = app.Browser.OpenURL(verificationURL)
	}
	interval := authorization.IntervalSeconds
	if interval < 1 {
		interval = 2
	}
	return DeviceLogin{
		ServerURL: normalized, DeviceCode: authorization.DeviceCode,
		UserCode: authorization.UserCode, VerificationURL: verificationURL,
		ExpiresAt: authorization.ExpiresAt.UTC().Format(time.RFC3339Nano), IntervalSeconds: interval,
	}, nil
}

func (d *Desktop) PollDeviceLogin(serverURL, deviceCode string) (DeviceLoginResult, error) {
	normalized, err := userconfig.NormalizeServerURL(serverURL)
	if err != nil {
		return DeviceLoginResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exchange, err := pactclient.ExchangeDeviceAuthorization(ctx, normalized, strings.TrimSpace(deviceCode))
	if err != nil {
		return DeviceLoginResult{}, fmt.Errorf("exchange device authorization: %w", err)
	}
	result := DeviceLoginResult{Status: exchange.Status}
	if exchange.ExpiresAt != nil {
		result.ExpiresAt = exchange.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if exchange.Status == "pending" {
		return result, nil
	}
	if exchange.Status != "authorized" || exchange.DeviceCredential == "" {
		return result, nil
	}
	client, err := pactclient.New(normalized, exchange.DeviceCredential)
	if err != nil {
		return DeviceLoginResult{}, err
	}
	principal, err := client.Me(ctx)
	if err != nil {
		return DeviceLoginResult{}, fmt.Errorf("verify authorized device: %w", err)
	}
	if _, err := userconfig.Save(normalized, exchange.DeviceCredential); err != nil {
		return DeviceLoginResult{}, err
	}
	result.Connected = true
	result.Principal = &principal
	return result, nil
}

func (d *Desktop) OpenExternalURL(address string) error {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return errors.New("external URL must be an absolute safe HTTP or HTTPS URL")
	}
	app := d.application()
	if app == nil {
		return errors.New("desktop window is not ready")
	}
	return app.Browser.OpenURL(parsed.String())
}

func (d *Desktop) Disconnect(localOnly bool) error {
	d.stopAllStreams()
	config, err := userconfig.Load()
	if err == nil && !localOnly {
		client, clientErr := pactclient.New(config.ServerURL, config.DeviceCredential)
		if clientErr != nil {
			return clientErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		revokeErr := client.RevokeCurrentDevice(ctx)
		cancel()
		if revokeErr != nil {
			return fmt.Errorf("revoke desktop device: %w", revokeErr)
		}
	}
	return userconfig.Delete()
}

func (d *Desktop) application() *application.App {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.app
}

func desktopDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "This computer"
	}
	return hostname + " (PACT Desktop)"
}

func resolveURL(serverURL, reference string) (string, error) {
	base, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	relative, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return "", fmt.Errorf("parse verification URL: %w", err)
	}
	resolved := base.ResolveReference(relative)
	if (resolved.Scheme != "http" && resolved.Scheme != "https") || resolved.Host == "" || resolved.User != nil {
		return "", errors.New("Pact Server returned an invalid verification URL")
	}
	if !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
		return "", errors.New("Pact Server returned a verification URL on another origin")
	}
	return resolved.String(), nil
}
