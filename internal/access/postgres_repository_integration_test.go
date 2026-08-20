//go:build integration

package access_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/authn"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/useradmin"
)

func TestLocalAccountsInvitationsAndDeviceRevocationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{ApplicationName: "pact-authentication-integration-test"})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var organizationID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO identity.organizations (slug, name)
		VALUES ($1, $2)
		RETURNING id
	`, "auth-"+suffix, "Authentication "+suffix).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	projectResult, err := projects.NewService(
		organizationID,
		projects.NewPostgresRepository(pool),
	).Create(ctx, "access-project-"+suffix, projects.CreateInput{
		Name: "Access project " + suffix,
		Slug: "access-project-" + suffix,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	const setupCode = "integration-setup-code-long-enough"
	authentication := authn.NewService(authn.Config{
		OrganizationID: organizationID,
		SetupToken:     setupCode,
		PublicURL:      "https://pact.example.com",
	}, authn.NewPostgresRepository(pool))
	ownerSession, err := authentication.Setup(ctx, authn.SetupInput{
		SetupCode: setupCode,
		AccountInput: authn.AccountInput{
			DisplayName: "Integration owner", Email: "owner-" + suffix + "@example.com",
			Username: "owner." + suffix[len(suffix)-8:], Password: "a sufficiently long owner password",
		},
	}, authn.SessionMetadata{UserAgent: "integration-test"})
	if err != nil {
		t.Fatalf("create owner account: %v", err)
	}
	owner, err := authentication.AuthenticateWeb(ctx, ownerSession.SessionSecret)
	if err != nil {
		t.Fatalf("authenticate owner session: %v", err)
	}
	if owner.Principal.OrganizationRole != "owner" || owner.Principal.Bootstrap {
		t.Fatalf("owner principal = %#v", owner.Principal)
	}
	if _, err := authentication.Setup(ctx, authn.SetupInput{
		SetupCode: setupCode,
		AccountInput: authn.AccountInput{
			DisplayName: "Second owner", Email: "second-" + suffix + "@example.com",
			Username: "second." + suffix[len(suffix)-8:], Password: "a sufficiently long second owner password",
		},
	}, authn.SessionMetadata{}); !errors.Is(err, authn.ErrAlreadyConfigured) {
		t.Fatalf("second owner setup error = %v", err)
	}
	if _, err := authentication.Login(ctx, authn.LoginInput{
		Login: "owner." + suffix[len(suffix)-8:], Password: "incorrect but sufficiently long",
	}, authn.SessionMetadata{}); !errors.Is(err, authn.ErrInvalidCredentials) {
		t.Fatalf("invalid owner login error = %v", err)
	}
	loginSession, err := authentication.Login(ctx, authn.LoginInput{
		Login: "owner-" + suffix + "@example.com", Password: "a sufficiently long owner password",
	}, authn.SessionMetadata{UserAgent: "integration-login"})
	if err != nil {
		t.Fatalf("owner password login: %v", err)
	}
	if loggedIn, err := authentication.AuthenticateWeb(ctx, loginSession.SessionSecret); err != nil || loggedIn.Principal.ID != owner.Principal.ID {
		t.Fatalf("owner login session = %#v, %v", loggedIn, err)
	}

	authorization := access.NewService(organizationID, access.NewPostgresRepository(pool))
	created, err := authorization.CreateInvitation(ctx, owner.Principal, projectResult.Project.ID, access.CreateInvitationInput{
		Email: "collaborator-" + suffix + "@example.com", Role: "contributor", ExpiresAfter: time.Hour,
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	preview, err := authentication.PreviewInvitation(ctx, created.Secret)
	if err != nil || preview.ProjectID != projectResult.Project.ID {
		t.Fatalf("preview invitation = %#v, %v", preview, err)
	}
	collaboratorSession, err := authentication.RegisterInvitation(ctx, authn.InvitationRegistrationInput{
		Secret: created.Secret,
		AccountInput: authn.AccountInput{
			DisplayName: "Integration collaborator", Email: created.Invitation.Email,
			Username: "collab." + suffix[len(suffix)-8:], Password: "a sufficiently long collaborator password",
		},
	}, authn.SessionMetadata{UserAgent: "integration-test"})
	if err != nil {
		t.Fatalf("register invited account: %v", err)
	}
	if collaboratorSession.Acceptance.ProjectRole != "contributor" {
		t.Fatalf("invitation acceptance = %#v", collaboratorSession.Acceptance)
	}
	if _, err := authentication.RegisterInvitation(ctx, authn.InvitationRegistrationInput{
		Secret: created.Secret,
		AccountInput: authn.AccountInput{
			DisplayName: "Replay", Email: created.Invitation.Email,
			Username: "replay." + suffix[len(suffix)-8:], Password: "a sufficiently long replay password",
		},
	}, authn.SessionMetadata{}); !errors.Is(err, authn.ErrInvitationInvalid) {
		t.Fatalf("replayed invitation error = %v", err)
	}

	collaborator := collaboratorSession.Acceptance.Principal
	if err := authorization.RequireProjectRole(ctx, collaborator, projectResult.Project.ID, "contributor"); err != nil {
		t.Fatalf("collaborator authorization: %v", err)
	}
	deviceAuthorization, err := authentication.BeginDevice(ctx, authn.BeginDeviceInput{DeviceName: "Integration laptop"})
	if err != nil {
		t.Fatalf("begin device authorization: %v", err)
	}
	if err := authentication.ApproveDevice(ctx, collaborator, deviceAuthorization.UserCode); err != nil {
		t.Fatalf("approve device: %v", err)
	}
	exchange, err := authentication.ExchangeDevice(ctx, deviceAuthorization.DeviceCode)
	if err != nil || exchange.Status != "authorized" || exchange.DeviceCredential == "" {
		t.Fatalf("device exchange = %#v, %v", exchange, err)
	}
	device, err := authentication.AuthenticateDevice(ctx, exchange.DeviceCredential)
	if err != nil || device.Principal.ID != collaborator.ID {
		t.Fatalf("device principal = %#v, %v", device, err)
	}

	if err := authentication.ChangePassword(ctx, collaboratorSession.Session, authn.ChangePasswordInput{
		CurrentPassword: "a sufficiently long collaborator password",
		NewPassword:     "a different sufficiently long password",
	}); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := authentication.AuthenticateDevice(ctx, exchange.DeviceCredential); !errors.Is(err, authn.ErrUnauthorized) {
		t.Fatalf("device remained active after password change: %v", err)
	}

	agentSession, err := agentsession.NewService(
		organizationID,
		agentsession.NewPostgresRepository(pool),
	).Start(ctx, collaborator.ID, projectResult.Project.ID, agentsession.StartInput{
		NodeKey: "access-node-" + suffix, NodeName: "Access node",
		AgentName: "Access Codex", AgentType: "codex", ClientType: "codex", ObserveGit: true,
	})
	if err != nil {
		t.Fatalf("start access agent session: %v", err)
	}
	secondAgentSession, err := agentsession.NewService(
		organizationID,
		agentsession.NewPostgresRepository(pool),
	).Start(ctx, collaborator.ID, projectResult.Project.ID, agentsession.StartInput{
		NodeKey: "access-node-second-" + suffix, NodeName: "Second access node",
		AgentName: "codex-release-check", AgentType: "codex", ClientType: "codex-mcp", ObserveGit: true,
	})
	if err != nil {
		t.Fatalf("start second access agent session: %v", err)
	}
	if secondAgentSession.ActorID != agentSession.ActorID || secondAgentSession.ActorName != "Codex" {
		t.Fatalf("logical agent identity was not reused: first=%#v second=%#v", agentSession, secondAgentSession)
	}
	roster, err := authorization.GetProjectAccess(ctx, owner.Principal, projectResult.Project.ID)
	if err != nil {
		t.Fatalf("get project access: %v", err)
	}
	if !containsMember(roster.Members, owner.Principal.ID) || !containsMember(roster.Members, collaborator.ID) || containsMember(roster.Members, access.BootstrapPrincipalID) {
		t.Fatalf("project members = %#v", roster.Members)
	}
	if len(roster.Agents) != 1 || roster.Agents[0].AgentID != agentSession.ActorID || !roster.Agents[0].Connected || roster.Agents[0].SessionCount != 2 {
		t.Fatalf("project agents = %#v", roster.Agents)
	}

	userAdministration := useradmin.NewService(
		organizationID,
		useradmin.NewPostgresRepository(pool),
	)
	directory, err := userAdministration.Directory(ctx, owner.Principal)
	if err != nil {
		t.Fatalf("list organization users: %v", err)
	}
	if len(directory.Users) != 2 || len(directory.Invitations) != 0 {
		t.Fatalf("initial user directory = %#v", directory)
	}
	updatedCollaborator, err := userAdministration.SetProjectPermission(
		ctx, owner.Principal, collaborator.ID, projectResult.Project.ID, "viewer",
	)
	if err != nil {
		t.Fatalf("lower collaborator project permission: %v", err)
	}
	if len(updatedCollaborator.ProjectRoles) != 1 || updatedCollaborator.ProjectRoles[0].Role != "viewer" {
		t.Fatalf("updated collaborator permissions = %#v", updatedCollaborator.ProjectRoles)
	}
	if err := authorization.RequireProjectRole(ctx, collaborator, projectResult.Project.ID, "contributor"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("lowered project permission error = %v", err)
	}
	disabled := "disabled"
	updatedCollaborator, err = userAdministration.UpdateUser(
		ctx, owner.Principal, collaborator.ID, useradmin.UpdateUserInput{Status: &disabled},
	)
	if err != nil || updatedCollaborator.Status != "disabled" || updatedCollaborator.ActiveSessions != 0 {
		t.Fatalf("disabled collaborator = %#v, %v", updatedCollaborator, err)
	}
	if _, err := authentication.Login(ctx, authn.LoginInput{
		Login: created.Invitation.Email, Password: "a different sufficiently long password",
	}, authn.SessionMetadata{}); !errors.Is(err, authn.ErrInvalidCredentials) {
		t.Fatalf("disabled collaborator login error = %v", err)
	}
	var sponsoredSessionStatus string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM identity.sessions WHERE organization_id = $1 AND id = $2
	`, organizationID, agentSession.ID).Scan(&sponsoredSessionStatus); err != nil || sponsoredSessionStatus != "closed" {
		t.Fatalf("sponsored session status = %q, %v", sponsoredSessionStatus, err)
	}
	active := "active"
	if _, err := userAdministration.UpdateUser(
		ctx, owner.Principal, collaborator.ID, useradmin.UpdateUserInput{Status: &active},
	); err != nil {
		t.Fatalf("reactivate collaborator: %v", err)
	}

	adminInvitation, err := userAdministration.CreateInvitation(ctx, owner.Principal, useradmin.CreateInvitationInput{
		Email: "admin-" + suffix + "@example.com", OrganizationRole: "admin", ExpiresAfter: time.Hour,
	})
	if err != nil {
		t.Fatalf("create organization administrator invitation: %v", err)
	}
	adminPreview, err := authentication.PreviewInvitation(ctx, adminInvitation.Secret)
	if err != nil || adminPreview.OrganizationRole != "admin" || adminPreview.ProjectID != "" {
		t.Fatalf("administrator invitation preview = %#v, %v", adminPreview, err)
	}
	adminSession, err := authentication.RegisterInvitation(ctx, authn.InvitationRegistrationInput{
		Secret: adminInvitation.Secret,
		AccountInput: authn.AccountInput{
			DisplayName: "Integration administrator", Email: adminInvitation.Invitation.Email,
			Username: "admin." + suffix[len(suffix)-8:], Password: "a sufficiently long administrator password",
		},
	}, authn.SessionMetadata{UserAgent: "integration-test"})
	if err != nil || adminSession.Acceptance.Principal.OrganizationRole != "admin" {
		t.Fatalf("register organization administrator = %#v, %v", adminSession, err)
	}
	directory, err = userAdministration.Directory(ctx, owner.Principal)
	if err != nil || len(directory.Users) != 3 || len(directory.Events) < 4 {
		t.Fatalf("final user directory = %#v, %v", directory, err)
	}
}

func containsMember(members []access.ProjectMember, principalID string) bool {
	for _, member := range members {
		if member.PrincipalID == principalID {
			return true
		}
	}
	return false
}
