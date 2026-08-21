package httpapi

import (
	"errors"
	"net/http"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/repositorybinding"
)

func (a *API) handleResolveRepositoryBinding(w http.ResponseWriter, r *http.Request) {
	if a.repositoryBindings == nil || a.access == nil {
		a.writeDomainError(w, r, errors.New("repository binding resolution is not configured"))
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input repositorybinding.ResolveInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	matches, err := a.repositoryBindings.Resolve(r.Context(), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	principal, ok := principalFromContext(r.Context())
	if !ok {
		a.writeDomainError(w, r, access.ErrUnauthorized)
		return
	}
	visible, err := a.access.VisibleProjectIDs(r.Context(), principal)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	filtered := make([]repositorybinding.Match, 0, len(matches))
	for _, match := range matches {
		if visible != nil {
			if _, allowed := visible[match.ProjectID]; !allowed {
				continue
			}
		}
		projectAccess, accessErr := a.access.GetProjectAccess(r.Context(), principal, match.ProjectID)
		if accessErr != nil {
			if errors.Is(accessErr, access.ErrForbidden) || errors.Is(accessErr, access.ErrNotFound) {
				continue
			}
			a.writeDomainError(w, r, accessErr)
			return
		}
		match.Permission = bindingPermission(projectAccess, principal)
		filtered = append(filtered, match)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"matches": filtered}})
}

func bindingPermission(projectAccess access.ProjectAccess, principal access.Principal) string {
	for _, member := range projectAccess.Members {
		if member.PrincipalID == principal.ID {
			if member.EffectiveRole != "" {
				return member.EffectiveRole
			}
			if member.ProjectRole != "" {
				return member.ProjectRole
			}
		}
	}
	switch principal.OrganizationRole {
	case "owner":
		return "owner"
	case "admin":
		return "maintainer"
	default:
		return "viewer"
	}
}
