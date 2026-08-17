package httpapi

import (
	"net/http"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/useradmin"
)

func (a *API) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	directory, err := a.userAdmin.Directory(r.Context(), principal)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": directory})
}

func (a *API) handleAdminGetUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	user, err := a.userAdmin.GetUser(r.Context(), principal, r.PathValue("principalID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": user})
}

func (a *API) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	var input useradmin.UpdateUserInput
	if !a.decodeUserAdminJSON(w, r, &input) {
		return
	}
	user, err := a.userAdmin.UpdateUser(r.Context(), principal, r.PathValue("principalID"), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": user})
}

func (a *API) handleAdminDisableUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	status := "disabled"
	user, err := a.userAdmin.UpdateUser(
		r.Context(), principal, r.PathValue("principalID"),
		useradmin.UpdateUserInput{Status: &status},
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": user})
}

func (a *API) handleAdminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	user, err := a.userAdmin.RevokeUserSessions(r.Context(), principal, r.PathValue("principalID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": user})
}

func (a *API) handleAdminSetProjectPermission(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	if !a.decodeUserAdminJSON(w, r, &input) {
		return
	}
	user, err := a.userAdmin.SetProjectPermission(
		r.Context(), principal, r.PathValue("principalID"), r.PathValue("projectID"), input.Role,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": user})
}

func (a *API) handleAdminRemoveProjectPermission(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	user, err := a.userAdmin.RemoveProjectPermission(
		r.Context(), principal, r.PathValue("principalID"), r.PathValue("projectID"),
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": user})
}

func (a *API) handleAdminCreateInvitation(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	var input struct {
		Email            string `json:"email"`
		OrganizationRole string `json:"organization_role"`
		ProjectID        string `json:"project_id"`
		ProjectRole      string `json:"project_role"`
		ExpiresInHours   int    `json:"expires_in_hours"`
	}
	if !a.decodeUserAdminJSON(w, r, &input) {
		return
	}
	expiresAfter := time.Duration(input.ExpiresInHours) * time.Hour
	if input.ExpiresInHours == 0 {
		expiresAfter = 24 * time.Hour
	}
	created, err := a.userAdmin.CreateInvitation(r.Context(), principal, useradmin.CreateInvitationInput{
		Email: input.Email, OrganizationRole: input.OrganizationRole,
		ProjectID: input.ProjectID, ProjectRole: input.ProjectRole,
		ExpiresAfter: expiresAfter,
	})
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", "/v1/admin/invitations/"+created.Invitation.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (a *API) handleAdminRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.webAdministrator(w, r)
	if !ok {
		return
	}
	if err := a.userAdmin.RevokeInvitation(r.Context(), principal, r.PathValue("invitationID")); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) webAdministrator(w http.ResponseWriter, r *http.Request) (access.Principal, bool) {
	if a.userAdmin == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "user_administration_unavailable", "User administration unavailable", "Pact user administration is not configured.")
		return access.Principal{}, false
	}
	authentication, ok := authenticationFromContext(r.Context())
	if !ok || authentication.Web == nil {
		writeProblem(w, r, http.StatusForbidden, "web_session_required", "Web session required", "Organization user administration requires an interactive web session.")
		return access.Principal{}, false
	}
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "Unauthorized", "Authentication is required.")
		return access.Principal{}, false
	}
	return principal, true
}

func (a *API) decodeUserAdminJSON(w http.ResponseWriter, r *http.Request, target any) bool {
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
