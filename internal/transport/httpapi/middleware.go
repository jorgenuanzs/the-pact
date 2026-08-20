package httpapi

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/authn"
)

type requestIDKey struct{}
type principalKey struct{}
type authenticationKey struct{}

type requestAuthentication struct {
	Kind       string
	Credential string
	Web        *authn.WebSession
	Device     *authn.DevicePrincipal
}

func (a *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.authentication == nil {
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "Unauthorized", "Authentication is required.")
			return
		}

		var principal access.Principal
		var authentication requestAuthentication
		if cookie, err := r.Cookie(authn.WebSessionCookie); err == nil && strings.TrimSpace(cookie.Value) != "" {
			session, authErr := a.authentication.AuthenticateWeb(r.Context(), cookie.Value)
			if authErr != nil {
				a.writeDomainError(w, r, authErr)
				return
			}
			if methodRequiresCSRF(r.Method) {
				csrfCookie, cookieErr := r.Cookie(authn.CSRFCookie)
				csrfHeader := strings.TrimSpace(r.Header.Get("X-Pact-CSRF"))
				if cookieErr != nil || csrfHeader == "" || subtle.ConstantTimeCompare([]byte(csrfCookie.Value), []byte(csrfHeader)) != 1 || !a.authentication.ValidateCSRF(session, csrfHeader) {
					writeProblem(w, r, http.StatusForbidden, "csrf_invalid", "Request rejected", "The browser security token is missing or invalid.")
					return
				}
			}
			principal = session.Principal
			authentication = requestAuthentication{Kind: "web", Credential: cookie.Value, Web: &session}
		} else {
			scheme, credential, found := strings.Cut(r.Header.Get("Authorization"), " ")
			if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(credential) == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "Unauthorized", "Log in to Pact or provide a valid device credential.")
				return
			}
			device, authErr := a.authentication.AuthenticateDevice(r.Context(), credential)
			if authErr != nil {
				w.Header().Set("WWW-Authenticate", "Bearer")
				a.writeDomainError(w, r, authErr)
				return
			}
			principal = device.Principal
			authentication = requestAuthentication{Kind: "device", Credential: credential, Device: &device}
		}
		ctx := context.WithValue(r.Context(), principalKey{}, principal)
		ctx = context.WithValue(ctx, authenticationKey{}, authentication)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authenticationFromContext(ctx context.Context) (requestAuthentication, bool) {
	authentication, ok := ctx.Value(authenticationKey{}).(requestAuthentication)
	return authentication, ok
}

func methodRequiresCSRF(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func (a *API) requireProjectRole(minimum string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok || a.access == nil {
			a.writeDomainError(w, r, access.ErrUnauthorized)
			return
		}
		if err := a.access.RequireProjectRole(r.Context(), principal, r.PathValue("projectID"), minimum); err != nil {
			a.writeDomainError(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) requireWorkspaceRole(minimum string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok || a.access == nil || a.workspaces == nil {
			a.writeDomainError(w, r, access.ErrUnauthorized)
			return
		}
		workspace, err := a.workspaces.Get(r.Context(), r.PathValue("workspaceID"))
		if err != nil {
			a.writeDomainError(w, r, err)
			return
		}
		if a.access.CanCreateProject(principal) {
			next.ServeHTTP(w, r)
			return
		}
		for _, project := range workspace.Projects {
			err = a.access.RequireProjectRole(r.Context(), principal, project.ID, minimum)
			if err == nil {
				next.ServeHTTP(w, r)
				return
			}
			if err != access.ErrForbidden && err != access.ErrNotFound {
				a.writeDomainError(w, r, err)
				return
			}
		}
		a.writeDomainError(w, r, access.ErrForbidden)
	})
}

func principalFromContext(ctx context.Context) (access.Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(access.Principal)
	return principal, ok
}

func (a *API) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	count, err := r.ResponseWriter.Write(body)
	r.bytes += count
	return count, err
}

func (r *responseRecorder) Flush() {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (a *API) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		a.logger.LogAttrs(
			r.Context(),
			slog.LevelInfo,
			"http request",
			slog.String("request_id", requestIDFromContext(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", recorder.status),
			slog.Int("response_bytes", recorder.bytes),
			slog.Duration("duration", time.Since(started)),
		)
	})
}

func (a *API) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.ErrorContext(r.Context(), "panic recovered", "panic", recovered)
				if recorder, ok := w.(*responseRecorder); ok && recorder.status != 0 {
					return
				}
				writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal server error", "The request could not be completed.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
