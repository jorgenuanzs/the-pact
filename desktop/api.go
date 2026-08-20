package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/userconfig"
)

const maxDesktopResponse = 8 << 20

type DesktopAPIRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

type DesktopAPIResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func (d *Desktop) APIRequest(input DesktopAPIRequest) (DesktopAPIResponse, error) {
	config, err := userconfig.Load()
	if err != nil {
		return DesktopAPIResponse{}, err
	}
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return DesktopAPIResponse{}, errors.New("unsupported HTTP method")
	}
	requestURL, err := desktopEndpoint(config.ServerURL, input.Path)
	if err != nil {
		return DesktopAPIResponse{}, err
	}
	if len(input.Body) > maxDesktopResponse {
		return DesktopAPIResponse{}, errors.New("desktop request body is too large")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewBufferString(input.Body))
	if err != nil {
		return DesktopAPIResponse{}, fmt.Errorf("create desktop request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+config.DeviceCredential)
	request.Header.Set("Accept", headerOr(input.Headers, "Accept", "application/json"))
	for _, name := range []string{"Content-Type", "Idempotency-Key", "If-Match"} {
		if value := headerValue(input.Headers, name); value != "" {
			request.Header.Set(name, value)
		}
	}
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return DesktopAPIResponse{}, fmt.Errorf("contact Pact Server: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDesktopResponse+1))
	if err != nil {
		return DesktopAPIResponse{}, fmt.Errorf("read Pact Server response: %w", err)
	}
	if len(body) > maxDesktopResponse {
		return DesktopAPIResponse{}, errors.New("Pact Server response is too large")
	}
	return DesktopAPIResponse{
		Status:  response.StatusCode,
		Headers: map[string]string{"Content-Type": response.Header.Get("Content-Type")},
		Body:    string(body),
	}, nil
}

func desktopEndpoint(serverURL, requestPath string) (string, error) {
	path, err := url.ParseRequestURI(strings.TrimSpace(requestPath))
	if err != nil || path.IsAbs() || path.Host != "" || !strings.HasPrefix(path.Path, "/v1/") {
		return "", errors.New("desktop API path must remain inside /v1")
	}
	base, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("parse Pact Server URL: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + path.Path
	base.RawQuery = path.RawQuery
	return base.String(), nil
}

func headerOr(headers map[string]string, name, fallback string) string {
	if value := headerValue(headers, name); value != "" {
		return value
	}
	return fallback
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
