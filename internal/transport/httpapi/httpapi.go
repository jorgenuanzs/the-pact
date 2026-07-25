package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

const maxRequestBody = 1 << 20

type ProjectService interface {
	Create(context.Context, string, projects.CreateInput) (projects.CreateResult, error)
	Get(context.Context, string) (projects.Project, error)
}

type ReadinessCheck func(context.Context) error

type Config struct {
	Logger               *slog.Logger
	APIToken             string
	OrganizationID       string
	Build                buildinfo.Info
	Readiness            ReadinessCheck
	ProjectService       ProjectService
	EventReader          eventlog.Reader
	StreamShutdown       <-chan struct{}
	StreamPollInterval   time.Duration
	StreamHeartbeatEvery time.Duration
}

type API struct {
	logger               *slog.Logger
	tokenHash            [sha256.Size]byte
	organizationID       string
	build                buildinfo.Info
	readiness            ReadinessCheck
	projects             ProjectService
	events               eventlog.Reader
	streamShutdown       <-chan struct{}
	streamPollInterval   time.Duration
	streamHeartbeatEvery time.Duration
}

func New(cfg Config) http.Handler {
	if cfg.StreamPollInterval <= 0 {
		cfg.StreamPollInterval = time.Second
	}
	if cfg.StreamHeartbeatEvery <= 0 {
		cfg.StreamHeartbeatEvery = 15 * time.Second
	}

	api := &API{
		logger:               cfg.Logger,
		tokenHash:            sha256.Sum256([]byte(cfg.APIToken)),
		organizationID:       cfg.OrganizationID,
		build:                cfg.Build,
		readiness:            cfg.Readiness,
		projects:             cfg.ProjectService,
		events:               cfg.EventReader,
		streamShutdown:       cfg.StreamShutdown,
		streamPollInterval:   cfg.StreamPollInterval,
		streamHeartbeatEvery: cfg.StreamHeartbeatEvery,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", api.handleLive)
	mux.HandleFunc("GET /readyz", api.handleReady)
	mux.HandleFunc("GET /version", api.handleVersion)
	mux.Handle("POST /v1/projects", api.requireAuth(http.HandlerFunc(api.handleCreateProject)))
	mux.Handle("GET /v1/projects/{projectID}", api.requireAuth(http.HandlerFunc(api.handleGetProject)))
	mux.Handle("GET /v1/projects/{projectID}/events", api.requireAuth(http.HandlerFunc(api.handleListEvents)))
	mux.Handle("GET /v1/projects/{projectID}/events/stream", api.requireAuth(http.HandlerFunc(api.handleStreamEvents)))
	mux.Handle("/livez", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/readyz", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/version", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/events", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/events/stream", api.methodNotAllowed(http.MethodGet))
	mux.HandleFunc("/", api.handleNotFound)

	return api.requestContext(api.accessLog(api.recoverPanic(mux)))
}

func (a *API) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "live"}})
}

func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if a.readiness == nil || a.readiness(ctx) != nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "not_ready", "Service unavailable", "Pact is not ready to receive traffic.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ready"}})
}

func (a *API) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": a.build})
}

func (a *API) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}

	var input projects.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}

	result, err := a.projects.Create(r.Context(), r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/projects/"+result.Project.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": result.Project})
}

func (a *API) handleGetProject(w http.ResponseWriter, r *http.Request) {
	project, err := a.projects.Get(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": project})
}

func (a *API) handleListEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, err := a.projects.Get(r.Context(), projectID); err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	after, limit, err := eventPage(r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_cursor", "Invalid event cursor", err.Error())
		return
	}

	events, err := a.events.List(r.Context(), a.organizationID, projectID, after, limit+1)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	var nextCursor *string
	if len(events) > 0 {
		cursor := strconv.FormatInt(events[len(events)-1].ProjectSequence, 10)
		nextCursor = &cursor
	}

	responses := make([]eventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, newEventResponse(event))
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"events":      responses,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	}})
}

type eventResponse struct {
	ID               string          `json:"id"`
	ProjectID        string          `json:"project_id"`
	Sequence         string          `json:"sequence"`
	Type             string          `json:"type"`
	Version          int16           `json:"version"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      string          `json:"aggregate_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	CommandID        string          `json:"command_id"`
	CorrelationID    string          `json:"correlation_id"`
	ActorID          *string         `json:"actor_id,omitempty"`
	SessionID        *string         `json:"session_id,omitempty"`
	IntentID         *string         `json:"intent_id,omitempty"`
	CausationID      *string         `json:"causation_id,omitempty"`
	OccurredAt       time.Time       `json:"occurred_at"`
	RecordedAt       time.Time       `json:"recorded_at"`
	Data             json.RawMessage `json:"data"`
}

func newEventResponse(event eventlog.Event) eventResponse {
	return eventResponse{
		ID:               event.ID,
		ProjectID:        event.ProjectID,
		Sequence:         strconv.FormatInt(event.ProjectSequence, 10),
		Type:             event.Type,
		Version:          event.Version,
		AggregateType:    event.AggregateType,
		AggregateID:      event.AggregateID,
		AggregateVersion: event.AggregateVersion,
		CommandID:        event.CommandID,
		CorrelationID:    event.CorrelationID,
		ActorID:          event.ActorID,
		SessionID:        event.SessionID,
		IntentID:         event.IntentID,
		CausationID:      event.CausationID,
		OccurredAt:       event.OccurredAt,
		RecordedAt:       event.RecordedAt,
		Data:             event.Payload,
	}
}

func (a *API) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, err := a.projects.Get(r.Context(), projectID); err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	after, err := parseCursor(r.Header.Get("Last-Event-ID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_cursor", "Invalid event cursor", err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, r, http.StatusInternalServerError, "stream_unsupported", "Streaming unavailable", "The HTTP connection does not support streaming.")
		return
	}
	controller := http.NewResponseController(w)
	writeDeadlineSupported := true
	refreshWriteDeadline := func() bool {
		if !writeDeadlineSupported {
			return true
		}
		err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if errors.Is(err, http.ErrNotSupported) {
			writeDeadlineSupported = false
			return true
		}
		if err != nil {
			a.logger.WarnContext(r.Context(), "could not set stream write deadline", "error", err)
			return false
		}
		return true
	}
	if !refreshWriteDeadline() {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ": pact event stream\n\n")
	flusher.Flush()

	poll := time.NewTicker(a.streamPollInterval)
	defer poll.Stop()
	heartbeat := time.NewTicker(a.streamHeartbeatEvery)
	defer heartbeat.Stop()

	for {
		select {
		case <-a.streamShutdown:
			return
		default:
		}

		events, listErr := a.events.List(r.Context(), a.organizationID, projectID, after, 100)
		if listErr != nil {
			a.logger.ErrorContext(r.Context(), "event stream query failed", "error", listErr, "project_id", projectID)
			return
		}

		for _, event := range events {
			if !refreshWriteDeadline() {
				return
			}
			body, marshalErr := json.Marshal(newEventResponse(event))
			if marshalErr != nil {
				a.logger.ErrorContext(r.Context(), "event stream encoding failed", "error", marshalErr, "event_id", event.ID)
				return
			}
			if _, writeErr := fmt.Fprintf(
				w,
				"id: %d\nevent: %s\ndata: %s\n\n",
				event.ProjectSequence,
				sseEventName(event.Type),
				body,
			); writeErr != nil {
				return
			}
			after = event.ProjectSequence
		}
		if len(events) > 0 {
			flusher.Flush()
			continue
		}

		select {
		case <-r.Context().Done():
			return
		case <-a.streamShutdown:
			return
		case <-poll.C:
		case <-heartbeat.C:
			if !refreshWriteDeadline() {
				return
			}
			if _, writeErr := io.WriteString(w, ": keepalive\n\n"); writeErr != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (a *API) methodNotAllowed(allowed string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allowed)
		writeProblem(
			w,
			r,
			http.StatusMethodNotAllowed,
			"method_not_allowed",
			"Method not allowed",
			"The requested resource does not support this HTTP method.",
		)
	})
}

func (a *API) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeProblem(
		w,
		r,
		http.StatusNotFound,
		"route_not_found",
		"Route not found",
		"The requested API route does not exist.",
	)
}

func sseEventName(value string) string {
	if value == "" {
		return "pact.event"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' ||
			char == '_' ||
			char == '-' {
			continue
		}
		return "pact.event"
	}
	return value
}

func (a *API) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *projects.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", validationErr.Error())
	case errors.Is(err, projects.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "project_not_found", "Project not found", "The requested project does not exist.")
	case errors.Is(err, projects.ErrSlugTaken):
		writeProblem(w, r, http.StatusConflict, "project_slug_taken", "Project already exists", err.Error())
	case errors.Is(err, projects.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, projects.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	default:
		a.logger.ErrorContext(r.Context(), "request failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal server error", "The request could not be completed.")
	}
}

func eventPage(r *http.Request) (int64, int, error) {
	after, err := parseCursor(r.URL.Query().Get("after"))
	if err != nil {
		return 0, 0, err
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 200 {
			return 0, 0, errors.New("limit must be an integer between 1 and 200")
		}
		limit = value
	}
	return after, limit, nil
}

func parseCursor(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, errors.New("after and Last-Event-ID must be non-negative event cursors")
	}
	return cursor, nil
}

func hasJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "https://the-pact.dev/problems/" + code,
		"title":      title,
		"status":     status,
		"detail":     detail,
		"instance":   r.URL.Path,
		"code":       code,
		"request_id": requestIDFromContext(r.Context()),
	})
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func tokenMatches(expected [sha256.Size]byte, supplied string) bool {
	actual := sha256.Sum256([]byte(supplied))
	return subtle.ConstantTimeCompare(expected[:], actual[:]) == 1
}
