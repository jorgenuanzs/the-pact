package pactclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchVersionDoesNotRequireADeviceCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/version" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatal("version discovery must not send a device credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"version":"v0.17.0","commit":"abcdef","date":"2026-08-24T00:00:00Z","protocol_version":1,"min_protocol_version":1}}`))
	}))
	defer server.Close()

	info, err := FetchVersion(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch version: %v", err)
	}
	if info.Version != "v0.17.0" || info.ProtocolVersion != 1 || info.MinProtocolVersion != 1 {
		t.Fatalf("unexpected version: %+v", info)
	}
}
