package adminui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesControlPlane(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want HTML", contentType)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}

	body := response.Body.String()
	for _, marker := range []string{
		`id="workspace-overview"`,
		`id="workspace-rooms-title"`,
		`id="room-message-form"`,
		`id="project-tabs"`,
		`data-dashboard-view="live"`,
		`id="attention-panel"`,
		`id="settings-title"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("control plane does not contain %q", marker)
		}
	}
}

func TestHandlerServesFrontendAssets(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
		marker      string
	}{
		{path: "/admin/app.js", contentType: "text/javascript; charset=utf-8", marker: "function renderRoomConversation"},
		{path: "/admin/styles.css", contentType: "text/css; charset=utf-8", marker: ".workspace-rooms-panel"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()

			Handler().ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != tt.contentType {
				t.Fatalf("Content-Type = %q, want %q", contentType, tt.contentType)
			}
			if !strings.Contains(response.Body.String(), tt.marker) {
				t.Fatalf("asset does not contain %q", tt.marker)
			}
		})
	}
}

func TestHandlerRejectsUnknownAdminAsset(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/admin/unknown.js", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
