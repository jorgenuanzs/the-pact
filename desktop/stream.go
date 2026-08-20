package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/userconfig"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const desktopStreamEvent = "pact:desktop-project-stream"

type DesktopStreamMessage struct {
	SubscriptionID string          `json:"subscription_id"`
	ProjectID      string          `json:"project_id"`
	Kind           string          `json:"kind"`
	Status         string          `json:"status,omitempty"`
	EventID        string          `json:"event_id,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	Error          string          `json:"error,omitempty"`
}

func (d *Desktop) StartProjectEventStream(projectID, cursor string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || len(projectID) > 200 || strings.ContainsAny(projectID, "\r\n") {
		return "", errors.New("project ID is required")
	}
	if _, err := userconfig.Load(); err != nil {
		return "", err
	}
	subscriptionID, err := randomStreamID()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.streams[subscriptionID] = cancel
	d.mu.Unlock()
	go d.runProjectEventStream(ctx, subscriptionID, projectID, strings.TrimSpace(cursor))
	return subscriptionID, nil
}

func (d *Desktop) StopProjectEventStream(subscriptionID string) {
	d.mu.Lock()
	cancel := d.streams[strings.TrimSpace(subscriptionID)]
	delete(d.streams, strings.TrimSpace(subscriptionID))
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (d *Desktop) stopAllStreams() {
	d.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(d.streams))
	for _, cancel := range d.streams {
		cancels = append(cancels, cancel)
	}
	d.streams = make(map[string]context.CancelFunc)
	d.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (d *Desktop) runProjectEventStream(ctx context.Context, subscriptionID, projectID, cursor string) {
	defer func() {
		d.mu.Lock()
		delete(d.streams, subscriptionID)
		d.mu.Unlock()
	}()
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second}
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		status := "connecting"
		if attempt > 0 {
			status = "reconnecting"
		}
		d.emitStream(DesktopStreamMessage{SubscriptionID: subscriptionID, ProjectID: projectID, Kind: "status", Status: status})
		connectedAt, nextCursor, terminal, err := d.consumeProjectEvents(ctx, subscriptionID, projectID, cursor)
		if nextCursor != "" {
			cursor = nextCursor
		}
		if ctx.Err() != nil || terminal {
			return
		}
		message := DesktopStreamMessage{SubscriptionID: subscriptionID, ProjectID: projectID, Kind: "status", Status: "offline"}
		if err != nil {
			message.Error = err.Error()
		}
		d.emitStream(message)
		if time.Since(connectedAt) > 10*time.Second {
			attempt = 0
		}
		delay := delays[min(attempt, len(delays)-1)]
		attempt++
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (d *Desktop) consumeProjectEvents(
	ctx context.Context, subscriptionID, projectID, cursor string,
) (time.Time, string, bool, error) {
	config, err := userconfig.Load()
	if err != nil {
		return time.Now(), cursor, true, err
	}
	endpoint, err := desktopEndpoint(config.ServerURL, "/v1/projects/"+url.PathEscape(projectID)+"/events/stream")
	if err != nil {
		return time.Now(), cursor, true, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return time.Now(), cursor, true, err
	}
	request.Header.Set("Authorization", "Bearer "+config.DeviceCredential)
	request.Header.Set("Accept", "text/event-stream")
	if cursor != "" {
		request.Header.Set("Last-Event-ID", cursor)
	}
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		return time.Now(), cursor, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		d.emitStream(DesktopStreamMessage{
			SubscriptionID: subscriptionID, ProjectID: projectID, Kind: "status",
			Status: "offline", Error: "La autorización del dispositivo ya no es válida.",
		})
		return time.Now(), cursor, true, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return time.Now(), cursor, false, fmt.Errorf("Pact event stream returned HTTP %d", response.StatusCode)
	}
	connectedAt := time.Now()
	d.emitStream(DesktopStreamMessage{SubscriptionID: subscriptionID, ProjectID: projectID, Kind: "status", Status: "connected"})
	nextCursor, err := readSSE(response.Body, cursor, func(eventID string, data json.RawMessage) {
		d.emitStream(DesktopStreamMessage{
			SubscriptionID: subscriptionID, ProjectID: projectID, Kind: "event",
			EventID: eventID, Data: data,
		})
	})
	return connectedAt, nextCursor, false, err
}

func readSSE(reader io.Reader, cursor string, emit func(string, json.RawMessage)) (string, error) {
	const maxEventSize = 1 << 20
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxEventSize)
	var eventID string
	data := make([]string, 0, 1)
	flush := func() {
		if len(data) == 0 {
			eventID = ""
			return
		}
		body := strings.Join(data, "\n")
		if json.Valid([]byte(body)) {
			if eventID != "" {
				cursor = eventID
			}
			emit(eventID, json.RawMessage(body))
		}
		eventID = ""
		data = data[:0]
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			field, value = line, ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			if !strings.ContainsRune(value, '\x00') {
				eventID = value
			}
		case "data":
			data = append(data, value)
		}
	}
	flush()
	return cursor, scanner.Err()
}

func (d *Desktop) emitStream(message DesktopStreamMessage) {
	if ctx := d.appContext(); ctx != nil {
		runtime.EventsEmit(ctx, desktopStreamEvent, message)
	}
}

func randomStreamID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create stream ID: %w", err)
	}
	return "stream-" + hex.EncodeToString(value[:]), nil
}
