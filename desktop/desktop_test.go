package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDesktopEndpointStaysOnConfiguredServer(t *testing.T) {
	endpoint, err := desktopEndpoint("https://pact.example.com/control", "/v1/workspaces?q=active")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://pact.example.com/control/v1/workspaces?q=active" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	for _, unsafe := range []string{
		"https://other.example.com/v1/workspaces",
		"/admin/",
		"/v2/workspaces",
		"javascript:alert(1)",
	} {
		if _, err := desktopEndpoint("https://pact.example.com", unsafe); err == nil {
			t.Errorf("desktopEndpoint accepted %q", unsafe)
		}
	}
}

func TestResolveURLRequiresServerOrigin(t *testing.T) {
	resolved, err := resolveURL("https://pact.example.com", "/admin/#device=ABCD")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "https://pact.example.com/admin/#device=ABCD" {
		t.Fatalf("resolved = %q", resolved)
	}
	if _, err := resolveURL("https://pact.example.com", "https://evil.example/admin/"); err == nil {
		t.Fatal("resolveURL accepted another origin")
	}
}

func TestReadSSEPreservesCursorAndJSON(t *testing.T) {
	input := strings.NewReader(": heartbeat\n\nid: 41\ndata: {\"sequence\":41,\ndata: \"type\":\"pact.intent.started.v1\"}\n\n")
	var eventID string
	var payload map[string]any
	cursor, err := readSSE(input, "40", func(id string, data json.RawMessage) {
		eventID = id
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "41" || eventID != "41" || payload["type"] != "pact.intent.started.v1" {
		t.Fatalf("cursor=%q id=%q payload=%v", cursor, eventID, payload)
	}
}
