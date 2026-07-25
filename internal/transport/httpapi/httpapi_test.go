package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

const testToken = "this-is-a-long-local-test-token"

type fakeProjectService struct {
	create func(context.Context, string, projects.CreateInput) (projects.CreateResult, error)
	get    func(context.Context, string) (projects.Project, error)
}

func (f fakeProjectService) Create(ctx context.Context, key string, input projects.CreateInput) (projects.CreateResult, error) {
	return f.create(ctx, key, input)
}

func (f fakeProjectService) Get(ctx context.Context, projectID string) (projects.Project, error) {
	return f.get(ctx, projectID)
}

type fakeEventReader struct {
	list func(context.Context, string, string, int64, int) ([]eventlog.Event, error)
}

func (f fakeEventReader) List(ctx context.Context, organizationID, projectID string, after int64, limit int) ([]eventlog.Event, error) {
	return f.list(ctx, organizationID, projectID, after, limit)
}

func TestLiveDoesNotRequireAuthentication(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID is empty")
	}
}

func TestProtectedEndpointRequiresToken(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})
	request := httptest.NewRequest(http.MethodPost, "/v1/projects", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q", response.Header().Get("WWW-Authenticate"))
	}
}

func TestUnknownRoutesAndMethodsUseProblemDetails(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantAllow  string
	}{
		{
			name:       "unknown route",
			method:     http.MethodGet,
			target:     "/unknown",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "known route with unsupported method",
			method:     http.MethodPost,
			target:     "/livez",
			wantStatus: http.StatusMethodNotAllowed,
			wantAllow:  http.MethodGet,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
				t.Fatalf("Content-Type = %q", contentType)
			}
			if allow := response.Header().Get("Allow"); allow != test.wantAllow {
				t.Fatalf("Allow = %q", allow)
			}
		})
	}
}

func TestCreateProjectRejectsUnknownJSONField(t *testing.T) {
	service := fakeProjectService{
		create: func(context.Context, string, projects.CreateInput) (projects.CreateResult, error) {
			t.Fatal("service should not be called")
			return projects.CreateResult{}, nil
		},
	}
	handler := testHandler(t, service, fakeEventReader{})
	request := authenticatedRequest(http.MethodPost, "/v1/projects", `{"name":"Pact","slug":"pact","surprise":true}`)
	request.Header.Set("Idempotency-Key", "create-pact")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestCreateProjectReturnsStoredResponseOnReplay(t *testing.T) {
	project := projects.Project{
		ID:             "018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Name:           "Pact",
		Slug:           "pact",
		Version:        1,
		CreatedAt:      time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	service := fakeProjectService{
		create: func(_ context.Context, key string, input projects.CreateInput) (projects.CreateResult, error) {
			if key != "create-pact" || input.Slug != "pact" {
				t.Fatalf("unexpected command: key=%q input=%#v", key, input)
			}
			return projects.CreateResult{Project: project, Replayed: true}, nil
		},
	}
	handler := testHandler(t, service, fakeEventReader{})
	request := authenticatedRequest(http.MethodPost, "/v1/projects", `{"name":"Pact","slug":"pact"}`)
	request.Header.Set("Idempotency-Key", "create-pact")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("Idempotency-Replayed = %q", response.Header().Get("Idempotency-Replayed"))
	}
	if response.Header().Get("Location") != "/v1/projects/"+project.ID {
		t.Fatalf("Location = %q", response.Header().Get("Location"))
	}
}

func TestListEventsPassesCursorAndLimit(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	service := fakeProjectService{
		get: func(_ context.Context, id string) (projects.Project, error) {
			if id != projectID {
				t.Fatalf("project ID = %q", id)
			}
			return projects.Project{ID: id}, nil
		},
	}
	reader := fakeEventReader{
		list: func(_ context.Context, organizationID, id string, after int64, limit int) ([]eventlog.Event, error) {
			if organizationID != "00000000-0000-4000-8000-000000000001" || id != projectID || after != 41 || limit != 21 {
				t.Fatalf("List(org=%q, project=%q, after=%d, limit=%d)", organizationID, id, after, limit)
			}
			return []eventlog.Event{{ProjectSequence: 42, Type: "project.created"}}, nil
		},
	}
	handler := testHandler(t, service, reader)
	request := authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/events?after=41&limit=20", "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			NextCursor *string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.NextCursor == nil || *body.Data.NextCursor != "42" {
		t.Fatalf("next_cursor = %v", body.Data.NextCursor)
	}
}

func TestStreamReplaysFromLastEventIDAndStopsOnCancellation(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	ctx, cancel := context.WithCancel(context.Background())
	service := fakeProjectService{
		get: func(_ context.Context, id string) (projects.Project, error) {
			return projects.Project{ID: id}, nil
		},
	}
	call := 0
	reader := fakeEventReader{
		list: func(_ context.Context, _, _ string, after int64, _ int) ([]eventlog.Event, error) {
			call++
			switch call {
			case 1:
				if after != 41 {
					t.Fatalf("first cursor = %d", after)
				}
				return []eventlog.Event{{
					ID:               "018f784a-68c1-7b0f-8f2a-cfc255f99e1e",
					ProjectID:        projectID,
					ProjectSequence:  42,
					Type:             "pact.project.created.v1",
					Version:          1,
					AggregateType:    "project",
					AggregateID:      projectID,
					AggregateVersion: 1,
					CommandID:        "018f784a-68c1-7b0f-8f2a-cfc255f99e1f",
					CorrelationID:    "018f784a-68c1-7b0f-8f2a-cfc255f99e1f",
					OccurredAt:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
					RecordedAt:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
					Payload:          json.RawMessage(`{"name":"Pact"}`),
				}}, nil
			default:
				if after != 42 {
					t.Fatalf("resumed cursor = %d", after)
				}
				cancel()
				return nil, nil
			}
		},
	}
	handler := testHandler(t, service, reader)
	request := authenticatedRequest(
		http.MethodGet,
		"/v1/projects/"+projectID+"/events/stream",
		"",
	).WithContext(ctx)
	request.Header.Set("Last-Event-ID", "41")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		"id: 42\n",
		"event: pact.project.created.v1\n",
		`"sequence":"42"`,
		`"data":{"name":"Pact"}`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("stream body does not contain %q: %s", expected, body)
		}
	}
}

func TestPanicIsRecoveredWithRequestID(t *testing.T) {
	service := fakeProjectService{
		get: func(context.Context, string) (projects.Project, error) {
			panic("unexpected failure")
		},
	}
	handler := testHandler(t, service, fakeEventReader{})
	request := authenticatedRequest(
		http.MethodGet,
		"/v1/projects/018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
		"",
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RequestID == "" || body.RequestID != response.Header().Get("X-Request-ID") {
		t.Fatalf("request_id = %q, header = %q", body.RequestID, response.Header().Get("X-Request-ID"))
	}
}

func TestSSEEventNameRejectsProtocolInjection(t *testing.T) {
	if got := sseEventName("project.created\nid: forged"); got != "pact.event" {
		t.Fatalf("sseEventName() = %q", got)
	}
	if got := sseEventName("pact.project.created.v1"); got != "pact.project.created.v1" {
		t.Fatalf("sseEventName() = %q", got)
	}
}

func testHandler(t *testing.T, projectService ProjectService, eventReader eventlog.Reader) http.Handler {
	t.Helper()
	return New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		APIToken:       testToken,
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"},
		Readiness:      func(context.Context) error { return nil },
		ProjectService: projectService,
		EventReader:    eventReader,
	})
}

func authenticatedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}
