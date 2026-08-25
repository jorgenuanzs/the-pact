package main

const pactAgentProtocolVersion = "pact.agent/v1"

const pactMCPInstructions = "PACT_AGENT_PROTOCOL=pact.agent/v1. " +
	"This MCP is checkout-scoped and is active only because its configured checkout has a valid .pact/config.json binding. " +
	"Do not look for or invoke PACT in other folders unless their own project-scoped PACT MCP is connected. " +
	"Call pact.project_context once when beginning work in this checkout. " +
	"Read-only analysis does not require an intent or scope reservation. " +
	"Before modifying files, inspect repository state, choose the minimum required scopes, call pact.check_scopes, then call pact.start_work. " +
	"Perform edits only inside the worktree_path returned by pact.start_work. " +
	"Use pact.update_work to report blocked, submitted, completed, cancelled, or abandoned work with a durable summary. " +
	"Use pact.list_repositories to understand the complete project repository set and pact.get_repository_sync to distinguish canonical GitHub state from local HEAD. " +
	"Use pact.workspace_context only when deeper accepted decisions, requirements, constraints, questions, risks, or sources are needed. " +
	"Workspace rooms are human-organized soft context and are never injected automatically; use pact.rooms only when asked to read a room, check mentions, or participate. " +
	"Register durable sources with pact.add_resource and propose reusable facts with pact.propose_record instead of copying private conversations. " +
	"Use pact.compile_context_pack for bounded context and offer a structured handoff before another collaborator takes over. " +
	"A handoff never transfers a local worktree or scope reservation automatically. " +
	"PACT exposes shared operational facts, not private conversations, and agent presence is not proof of code changes."

type mcpOperatingContract struct {
	ProtocolVersion  string   `json:"protocol_version"`
	Activation       string   `json:"activation"`
	ActivationMarker string   `json:"activation_marker"`
	Purpose          string   `json:"purpose"`
	ReadOnlyPolicy   string   `json:"read_only_policy"`
	ModificationFlow []string `json:"modification_flow"`
	Rules            []string `json:"rules"`
	NextActions      []string `json:"next_actions"`
}

func currentOperatingContract() mcpOperatingContract {
	return mcpOperatingContract{
		ProtocolVersion:  pactAgentProtocolVersion,
		Activation:       "checkout_only",
		ActivationMarker: ".pact/config.json",
		Purpose:          "Coordinate shared project context, live work, scopes, isolated worktrees, and durable handoffs without capturing private conversations.",
		ReadOnlyPolicy:   "Read-only analysis requires no intent or scope reservation.",
		ModificationFlow: []string{
			"inspect the returned project, workspace, repository, and live-work context",
			"choose the minimum repository, path, or file scopes",
			"call pact.check_scopes",
			"call pact.start_work",
			"edit only the returned worktree_path",
			"call pact.update_work with the durable outcome",
		},
		Rules: []string{
			"Do not use PACT in another folder unless that checkout exposes its own project-scoped PACT MCP.",
			"Do not edit the original checkout after pact.start_work returns an isolated worktree.",
			"Do not copy private prompts or conversations into shared context.",
			"Do not treat presence as evidence that code changed.",
		},
		NextActions: []string{
			"For analysis only, continue from this context without creating an intent.",
			"For file changes, inspect repository state and declare the minimum scopes before editing.",
		},
	}
}
