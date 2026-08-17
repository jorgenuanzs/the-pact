package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/authn"
)

func (a *API) handleAuthSetupStatus(w http.ResponseWriter, r *http.Request) {
	if a.authentication == nil {
		a.writeDomainError(w, r, errors.New("authentication service is not configured"))
		return
	}
	status, err := a.authentication.SetupStatus(r.Context())
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": status})
}

func (a *API) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	var input authn.SetupInput
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	created, err := a.authentication.Setup(r.Context(), input, authenticationMetadata(r))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	a.setAuthenticationCookies(w, r, created.SessionSecret, created.CSRFSecret, created.Session.ExpiresAt)
	writeJSON(w, http.StatusCreated, map[string]any{"data": created.Session})
}

func (a *API) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var input authn.LoginInput
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	created, err := a.authentication.Login(r.Context(), input, authenticationMetadata(r))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	a.setAuthenticationCookies(w, r, created.SessionSecret, created.CSRFSecret, created.Session.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"data": created.Session})
}

func (a *API) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	authentication, _ := authenticationFromContext(r.Context())
	principal, _ := principalFromContext(r.Context())
	data := map[string]any{"kind": authentication.Kind, "principal": principal}
	if authentication.Web != nil {
		data["expires_at"] = authentication.Web.ExpiresAt
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (a *API) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	authentication, ok := authenticationFromContext(r.Context())
	if !ok || authentication.Web == nil {
		writeProblem(w, r, http.StatusBadRequest, "web_session_required", "Web session required", "This operation requires a browser session.")
		return
	}
	if err := a.authentication.LogoutWeb(r.Context(), *authentication.Web); err != nil && !errors.Is(err, authn.ErrNotFound) {
		a.writeDomainError(w, r, err)
		return
	}
	a.clearAuthenticationCookies(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	authentication, ok := authenticationFromContext(r.Context())
	if !ok || authentication.Web == nil {
		writeProblem(w, r, http.StatusBadRequest, "web_session_required", "Web session required", "Password changes require a browser session.")
		return
	}
	var input authn.ChangePasswordInput
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	if err := a.authentication.ChangePassword(r.Context(), *authentication.Web, input); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleAuthInvitationPreview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Secret string `json:"secret"`
	}
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	preview, err := a.authentication.PreviewInvitation(r.Context(), input.Secret)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": preview})
}

func (a *API) handleAuthInvitationRegister(w http.ResponseWriter, r *http.Request) {
	var input authn.InvitationRegistrationInput
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	created, err := a.authentication.RegisterInvitation(r.Context(), input, authenticationMetadata(r))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	a.setAuthenticationCookies(w, r, created.SessionSecret, created.CSRFSecret, created.Session.ExpiresAt)
	writeJSON(w, http.StatusCreated, map[string]any{"data": created.Acceptance})
}

func (a *API) handleAuthInvitationAccept(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Secret string `json:"secret"`
	}
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	principal, _ := principalFromContext(r.Context())
	accepted, err := a.authentication.AcceptInvitation(r.Context(), principal, input.Secret)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": accepted})
}

func (a *API) handleAuthBeginDevice(w http.ResponseWriter, r *http.Request) {
	var input authn.BeginDeviceInput
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	authorization, err := a.authentication.BeginDevice(r.Context(), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"data": authorization})
}

func (a *API) handleAuthApproveDevice(w http.ResponseWriter, r *http.Request) {
	authentication, ok := authenticationFromContext(r.Context())
	if !ok || authentication.Web == nil {
		writeProblem(w, r, http.StatusBadRequest, "web_session_required", "Web session required", "Device authorization requires a browser session.")
		return
	}
	var input struct {
		UserCode string `json:"user_code"`
	}
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	if err := a.authentication.ApproveDevice(r.Context(), authentication.Web.Principal, input.UserCode); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleAuthExchangeDevice(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DeviceCode string `json:"device_code"`
	}
	if !a.decodeAuthenticationJSON(w, r, &input) {
		return
	}
	exchange, err := a.authentication.ExchangeDevice(r.Context(), input.DeviceCode)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": exchange})
}

func (a *API) handleAuthListDevices(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	devices, err := a.authentication.ListDevices(r.Context(), principal)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"devices": devices}})
}

func (a *API) handleAuthRevokeDevice(w http.ResponseWriter, r *http.Request) {
	authentication, ok := authenticationFromContext(r.Context())
	if !ok || authentication.Web == nil {
		writeProblem(w, r, http.StatusBadRequest, "web_session_required", "Web session required", "Managing devices requires a browser session.")
		return
	}
	if err := a.authentication.RevokeDevice(r.Context(), authentication.Web.Principal, r.PathValue("deviceID")); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleAuthRevokeCurrentDevice(w http.ResponseWriter, r *http.Request) {
	authentication, ok := authenticationFromContext(r.Context())
	if !ok || authentication.Device == nil {
		writeProblem(w, r, http.StatusBadRequest, "device_credential_required", "Device credential required", "This operation requires the current device credential.")
		return
	}
	if err := a.authentication.RevokeCurrentDevice(r.Context(), *authentication.Device); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) decodeAuthenticationJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if a.authentication == nil {
		a.writeDomainError(w, r, errors.New("authentication service is not configured"))
		return false
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return false
	}
	if err := decodeJSON(w, r, target); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return false
	}
	return true
}

func (a *API) setAuthenticationCookies(w http.ResponseWriter, r *http.Request, sessionSecret, csrfSecret string, expiresAt time.Time) {
	w.Header().Set("Cache-Control", "no-store")
	secure := requestIsSecure(r)
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name: authn.WebSessionCookie, Value: sessionSecret, Path: "/",
		Secure: secure, HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Expires: expiresAt, MaxAge: maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name: authn.CSRFCookie, Value: csrfSecret, Path: "/",
		Secure: secure, HttpOnly: false, SameSite: http.SameSiteStrictMode,
		Expires: expiresAt, MaxAge: maxAge,
	})
}

func (a *API) clearAuthenticationCookies(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{authn.WebSessionCookie, authn.CSRFCookie} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", Secure: requestIsSecure(r),
			HttpOnly: name == authn.WebSessionCookie, SameSite: http.SameSiteStrictMode,
			Expires: time.Unix(1, 0), MaxAge: -1,
		})
	}
}

func authenticationMetadata(r *http.Request) authn.SessionMetadata {
	remoteAddress := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddress); err == nil {
		remoteAddress = host
	}
	return authn.SessionMetadata{UserAgent: r.UserAgent(), RemoteAddress: remoteAddress}
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	return strings.EqualFold(forwarded, "https")
}
