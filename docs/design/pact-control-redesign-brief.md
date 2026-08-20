# PACT Control — comprehensive product redesign brief

> Copy everything below into Claude Design and attach screenshots of the current PACT interface. The screenshots are evidence of existing features and content, not a visual style to preserve.

## Your role

Act as a senior product designer and design-systems architect. Redesign **PACT Control**, the web control plane for PACT. Produce a coherent, high-fidelity product interface, not a marketing website and not a collection of disconnected dashboard cards.

The result must feel calm, precise, trustworthy, fast, and intentionally designed for technical teams operating projects with both human contributors and AI coding agents.

All interface copy must be in **neutral Spanish**. The design rationale and annotations may be in English.

## What PACT is

PACT is a live coordination and shared-context layer for software projects where humans and multiple AI agents work at the same time.

Git remains the source of truth for code and history. PACT does not replace Git, GitHub, IDEs, coding agents, or chat clients. It adds the missing coordination and memory layer around them:

- who is connected now;
- who or which agent is working;
- what outcome they are pursuing;
- which repositories, paths, or files they intend to modify;
- whether scopes overlap with someone else's work;
- which branch and isolated Git worktree belong to that work;
- what changed in the local checkout and canonical repository;
- decisions, requirements, risks, questions, evidence, and other durable project knowledge;
- structured handoffs between collaborators without copying their private chats;
- shared workspace conversations with explicit mentions;
- GitHub installations and the repositories attached to each project;
- people, agents, roles, sessions, devices, invitations, and audit events.

PACT Server is shared by an organization. People use the web app, while CLIs and AI clients connect through HTTP and MCP. The interface receives real-time project events through SSE and periodically refreshes selected live data.

## Domain model and vocabulary

Preserve this hierarchy and do not flatten concepts that have different lifecycles:

```text
Organization
  └── Workspace
       ├── Conversations / Rooms
       ├── Durable knowledge and resources
       └── Project
            ├── Repositories
            ├── People and agent sessions
            ├── Coordinated work / Intents
            │    ├── Scope claims
            │    ├── Isolated Git worktree
            │    └── Handoffs and Context Packs
            └── Durable project events
```

Definitions:

- **Organization:** the company-level security and administration boundary.
- **Workspace:** a durable collaboration boundary that groups related projects, shared conversations, resources, and knowledge. A product may have separate frontend, backend, mobile, and infrastructure projects in one Workspace.
- **Project:** an operational unit coordinated by PACT. A project may contain multiple repositories.
- **Repository:** a GitHub repository linked to a project, with a purpose such as frontend, backend, mobile, infrastructure, or documentation. One can be primary; others can be required or optional.
- **Session:** the live presence of a person or AI agent on a connected computer.
- **Intent:** a declared unit of coordinated work with an objective, success criteria, responsible actor, state, base revision, and durable summary.
- **Scope:** a repository, path, or file reserved by an Intent. Scopes can reveal collisions before two agents edit the same code.
- **Worktree:** the isolated local Git checkout assigned to an Intent. It is not the same as a Workspace.
- **Room / Conversation:** durable, human-created soft context at Workspace level. Messages do not automatically become work or official knowledge.
- **Record:** a typed durable fact such as a decision, requirement, restriction, risk, question, incident, or validation. Records have states such as proposed, accepted, disputed, superseded, revoked, or rejected.
- **Resource:** a reference to a source or evidence; PACT stores its metadata and locator, not necessarily the source contents.
- **Handoff:** a structured offer that records completed work, remaining work, blockers, validations, and next steps. Accepting it acknowledges receipt; it does not transfer a filesystem path or uncommitted code.
- **Context Pack:** an immutable, verifiable snapshot of relevant project, workspace, Git, work, knowledge, and handoff context.

Important boundaries:

- Git remains canonical for code.
- GitHub integration is organization-level; each Project links only the repositories it needs.
- Conversations belong to a Workspace, even when entered from a Project. Clearly label that shared scope.
- PACT stores shared structured state, not private AI chat transcripts.
- Knowledge is not automatically true because it appeared in a conversation. It becomes durable through a Record lifecycle.
- A connected agent is not necessarily working. Active work requires a current session plus an active or blocked Intent.

## Primary users

1. **Owner / administrator**
   - installs and operates PACT Server;
   - connects GitHub;
   - invites people and manages organization roles;
   - assigns project access;
   - revokes sessions or devices;
   - audits access and operational events.

2. **Maintainer / technical lead**
   - observes projects and repositories;
   - sees concurrent work and scope collisions;
   - reviews project health, blockers, handoffs, and canonical revisions;
   - maintains Workspace conversations and durable knowledge.

3. **Contributor**
   - connects a CLI or AI tool;
   - joins projects they can access;
   - follows active work, conversations, context, and code status;
   - coordinates with other people and agents.

4. **Observer / stakeholder**
   - reads project status, activity, conversations, and accepted context without changing operational state.

## Problems in the current interface

The current UI exposes much of the product, but it feels visually fragmented and structurally flat:

- organization, Workspace, Project, and account administration compete in the same surface;
- very large headings consume space without improving orientation;
- too many cards have the same visual weight;
- the project view uses many horizontal tabs followed by a long stack of panels;
- configuration, operational status, activity, conversations, and permissions do not feel like separate modes;
- the left navigation does not yet behave like a clear persistent project explorer;
- tables, cards, dialogs, empty states, counters, status chips, and actions need one consistent component system;
- real-time signals can be difficult to distinguish from durable historical data;
- visual decoration competes with dense operational information.

Do not preserve the current layout merely because it exists. Preserve capabilities and domain meaning.

## Product experience principles

1. **Selection before detail.** A stable project explorer stays visible; selecting a Workspace or Project updates the main pane to its right.
2. **Overview before depth.** The default page answers what needs attention now. Detailed data lives in focused sections.
3. **Live and durable are visually distinct.** Presence and heartbeat signals look different from accepted knowledge and historical audit events.
4. **Progressive disclosure.** Show summaries first, then open rows in drawers or dedicated detail pages.
5. **Dense but breathable.** This is an operational tool, so favor compact rows, filters, and tables over large promotional cards.
6. **One clear action hierarchy.** Each view has one primary action at most. Secondary and destructive actions are visually quieter and appropriately separated.
7. **Status must not depend only on color.** Always combine color with text, iconography, shape, or label.
8. **No false affordances.** Do not invent backend actions. Knowledge and handoffs are currently mostly observed in the web UI; agents create and update much of this state through MCP.

## Recommended information architecture

### 1. Global application shell

Use a desktop-first application shell with:

- a compact top bar or upper sidebar header containing the PACT brand and organization switcher;
- global search / command entry;
- global mentions inbox with unread count;
- PACT Server connection status;
- account menu with identity, role, **Acceso y seguridad**, and **Cerrar sesión**;
- a persistent **left sidebar** approximately 260–288 px wide;
- a flexible main content pane on the right.

The left sidebar should include:

```text
PACT / Organization switcher
Search or command palette

Inicio
Menciones

WORKSPACES
  footfall
    Resumen del workspace
    Conversaciones
    PROYECTOS
      footfall-web       ● 2 activos
      footfall-api       ●
      footfall-infra
  another-workspace
    ...

ADMINISTRACIÓN (role-gated)
  Acceso y seguridad
  Integraciones
```

Workspace groups must be collapsible. Projects remain easy to scan and search. A selected Project gets a strong but quiet active state. Show small, useful signals beside a project: health, unread attention, or number of active actors. Do not overload each item with badges.

Selecting a Workspace opens its overview in the right pane. Selecting a Project opens its Project shell in the right pane. Do not navigate to a disconnected visual universe.

### 2. Organization home

Create a concise landing view that answers:

- which Workspaces and Projects exist;
- where people or agents are active now;
- which projects need attention;
- recent organization-level activity;
- pending mentions or invitations, when relevant.

This should not duplicate every Project dashboard. It is a cross-project pulse and navigation surface.

### 3. Workspace overview

The Workspace page should contain:

- name, description, status, and concise counters;
- Project list with health and live activity;
- Workspace Conversations preview;
- durable context summary: accepted decisions, open questions, active risks, disputed records, and sources;
- recent Workspace-level changes;
- clear actions for creating a conversation where authorized.

Conversations and knowledge are first-class Workspace concepts. Make their scope obvious.

### 4. Project shell

Every Project page shares a compact header:

- breadcrumb: Workspace / Project;
- Project name and short description;
- operational health;
- connected/live actor avatars with accessible labels;
- primary repository and canonical branch / short commit;
- real-time stream state and last update;
- Project settings action.

Below the header, use a clear local navigation. Recommended sections:

1. **Resumen**
2. **Trabajo en vivo**
3. **Conversaciones**
4. **Contexto**
5. **Código**
6. **Actividad**
7. **Configuración**

Avoid seven equally loud pills. Use a restrained tab bar or compact secondary sidebar that makes the selected mode obvious and remains stable between pages.

### 5. Project — Resumen

This is the operational cockpit, not a full dump of all entities. It should answer, in order:

1. Is the Project healthy and synchronized?
2. Who is working now?
3. What work is active, blocked, or colliding?
4. What changed recently?
5. What needs a human decision?

Suggested structure:

- an **attention strip** for collisions, blocked Intents, disconnected observers, stale Git state, or required repositories that failed synchronization;
- a compact **Ahora** area with active people/agents and their current Intents;
- an **Active work** table with actor, objective, repository/scope, branch/worktree, state, and last heartbeat;
- a small **Repository health** summary;
- a **Recent activity** timeline limited to the most relevant events;
- a **Context needing review** summary for disputed records, open questions, risks, or pending handoffs.

Do not show four large numeric metric cards if a table or sentence communicates the information more directly.

### 6. Project — Trabajo en vivo

Design this as the most distinctive PACT surface.

Use a dense list or table of active and recent Intents. Each row should show:

- human or AI identity and avatar;
- agent/client name, such as Codex, Claude, or Kimi;
- work objective;
- state: active, blocked, submitted, completed, cancelled, or abandoned;
- claimed repositories, paths, or files;
- branch and isolated worktree;
- base revision and current observed revision;
- dirty/clean signal where available;
- last heartbeat;
- collision or overlap warning;
- handoff availability.

Clicking a row should open a detail drawer or dedicated detail pane showing success criteria, full scopes, validations, summary, events, worktree metadata, handoff, and Context Pack metadata. Keep the list visible when useful.

Visually distinguish:

- connected but idle;
- actively working;
- blocked;
- stale / heartbeat lost;
- scope overlap explicitly overridden;
- historical, no longer live work.

### 7. Project — Conversaciones

This page is a Project entry point into **Workspace-scoped rooms**. Include a persistent label such as “Compartido en Workspace footfall” so users understand the scope.

Use a conversation layout inspired by mature team messaging products:

- compact room directory with unread/mention state;
- selected room header with name, purpose, participants, and scope;
- message timeline with clear human vs agent identity;
- threaded replies or reply references;
- composer with explicit `@` mention selection;
- mentions inbox;
- empty, loading, permission, and disconnected states.

Do not make rooms look like AI prompt transcripts. They are shared team conversations. Mentioning an agent does not automatically execute it; the copy and states must not imply otherwise.

### 8. Project — Contexto

Separate informal conversation from durable knowledge.

Provide filters or grouped sections for:

- decisions;
- requirements;
- restrictions;
- risks;
- open questions;
- validations/incidents;
- sources/resources.

Each Record should visibly show type, lifecycle status, author, date, evidence count, and whether it is accepted, disputed, proposed, superseded, or revoked. Opening a Record should show its details and evidence relationships. Highlight disputed or stale knowledge without making every proposed item look like an alert.

Include a read-only area for Context Packs and Handoffs when available. Do not fabricate web editing actions that the current product does not support.

### 9. Project — Código

Support multi-repository projects as a first-class concept.

Show repositories in a compact table/list with:

- repository name and provider;
- purpose, such as frontend, backend, infra, mobile, or docs;
- primary / required / optional labels;
- canonical branch and commit;
- synchronization state and last check;
- GitHub installation/account;
- action to verify/synchronize when authorized.

GitHub connection is organization-level. This view maps authorized repositories to the current Project; it must not imply that each Project owns a separate GitHub login.

Use a clear empty/onboarding state when GitHub is not connected:

```text
1. Conectar GitHub para la organización
2. Autorizar repositorios en GitHub
3. Vincular uno o más repositorios a este proyecto
4. Elegir el repositorio principal
```

### 10. Project — Actividad

Design an auditable event timeline with filters for:

- actor;
- event type;
- repository;
- Intent;
- state/severity;
- time range.

Show live arrival subtly. Each event row needs a human-readable summary first and technical metadata second. Allow expanding raw identifiers and payload details without placing them in the primary scan path.

### 11. Project — Configuración

Keep this out of the daily operational flow. Organize it into calm subsections:

- General;
- Workspace membership;
- Access / Project roles;
- Repositories and GitHub mapping;
- integrations;
- advanced or destructive actions.

Destructive controls must live in a clearly separated danger area. Do not place routine actions next to destructive ones.

### 12. Acceso y seguridad

This is organization-level administration, not a Project dashboard section.

Provide top-level sections:

- **Usuarios**;
- **Invitaciones**;
- **Sesiones y dispositivos** where appropriate;
- **Auditoría**.

The Users view should be a full-width, filterable table showing identity, organization role, project access summary, status, sessions/devices, and last access. Selecting a person opens a side drawer or detail page with:

- Profile;
- Organization role;
- direct Project permissions;
- active web sessions and authorized devices;
- security actions;
- audit history.

Rules that must be represented in disabled states and explanatory copy:

- owners and admins have global Project access;
- direct Project permissions apply to members;
- nobody can remove or demote the last active owner;
- a user cannot disable their own account from this surface;
- deleting history is not offered; accounts are deactivated to preserve attribution;
- revoking sessions/devices is distinct from deactivating a user.

Invitation creation is a focused dialog or drawer. It includes email, organization role, optional initial Project, Project role, and expiration. After creation, show the one-time invitation URL with a copy action and a clear warning that it will not be shown again.

## Critical user flows to prototype

### Flow A — Select and inspect a Project

1. User logs in.
2. The sidebar shows Workspaces and nested Projects.
3. User selects `footfall`.
4. The right pane opens Project Resumen without replacing the global shell.
5. User immediately sees health, people/agents active now, active work, repository state, and items needing attention.
6. User opens a work row and inspects scopes, branch, worktree, heartbeat, and summary in a detail drawer.

### Flow B — Detect concurrent work

1. Codex is modifying `backend/payments`.
2. Claude attempts overlapping work in a file inside that path.
3. The Project shows an explicit scope collision.
4. The UI explains who owns each scope, whether the claim is exclusive or shared, when the lease expires, and whether an override was used.
5. The user can navigate to both Intents without losing context.

### Flow C — Conversation and agent mention

1. User opens Project Conversaciones.
2. UI makes clear that the rooms belong to its Workspace.
3. User enters `#product-decisions`, replies to a message, and selects `@Claude` from the mention list.
4. The mention becomes a durable inbox item.
5. The UI does not claim that Claude has started executing; it shows pending/read/responded/dismissed state.

### Flow D — GitHub and multiple repositories

1. Organization owner connects the GitHub App.
2. GitHub controls which account and repositories are authorized.
3. Back in PACT, the user sees available repositories.
4. User maps frontend, backend, and infrastructure repositories to one Project.
5. User marks one repository as primary and sees independent synchronization states for all of them.

### Flow E — Invite and administer a person

1. Owner opens Acceso y seguridad from global navigation/account controls.
2. Owner creates an invitation with organization and optional Project role.
3. PACT returns a one-use URL.
4. After registration, the person appears in the user table.
5. Owner can change allowed roles, revoke sessions/devices, inspect audit history, or deactivate the account while preserving attribution.

### Flow F — Authorize a CLI/device

1. `pact login` displays a short user code and opens the PACT web URL.
2. User logs in if needed.
3. A focused approval dialog asks the user to compare the displayed code and authorize the named device/tool.
4. The CLI receives its own revocable device credential; the password is never copied into the CLI.

## Component system

Create a reusable design system covering:

- application shell and responsive sidebar;
- organization switcher;
- Workspace group and Project navigation item;
- command/search field;
- breadcrumbs and compact page headers;
- local section navigation;
- data tables with filtering, sorting, pagination/virtualization readiness, and row selection;
- detail drawers;
- status chips and live indicators;
- avatar stacks with human/agent distinction;
- attention banners;
- event timeline rows;
- conversation directory, message, thread reference, mention menu, and composer;
- Record and Resource rows;
- repository rows;
- forms, selects, checkboxes, buttons, menus, tooltips, dialogs, and destructive confirmations;
- skeletons, empty states, inline errors, permission states, stale-data states, and offline/reconnecting states;
- toasts only for transient confirmation, never as the only place important information appears.

Use consistent sizing and states across every area.

## Visual direction

Aim for **technical clarity with restrained personality**:

- dark ink navigation shell with a warm neutral or very light gray content surface;
- PACT acid-lime used sparingly for brand, active selection, or live/healthy signals—not as a border around everything;
- semantic green, amber, red, and blue with accessible contrast;
- compact modern sans-serif for UI; monospace only for branches, commits, paths, IDs, and codes;
- approximately 8 px spacing rhythm;
- restrained 6–10 px corner radii;
- subtle borders and almost no decorative shadow;
- consistent outline icon family rather than mixed symbols or emoji;
- typography sized for an application: avoid giant poster-style headings;
- high information density, but with alignment and grouping that make scanning effortless.

Avoid:

- generic SaaS landing-page aesthetics;
- glassmorphism, excessive gradients, or glowing neon;
- graph-paper backgrounds behind dense administration views;
- every section becoming a large card;
- oversized empty space and oversized headings;
- decorative microcopy in all-caps everywhere;
- ambiguous icon-only actions;
- horizontal scrolling for primary desktop flows;
- visualizing every count as a KPI card;
- making AI agents look magical or anthropomorphic. Treat them as accountable operational actors.

## Responsive behavior

Design desktop first at 1440 px width, then provide behavior for approximately 1280, 1024, and mobile widths.

- At desktop, the Workspace/Project explorer stays visible.
- At narrower widths, it becomes a collapsible drawer.
- Tables can progressively hide secondary columns, but the actor, objective, state, and attention signal remain visible.
- Detail drawers may become full-screen sheets on mobile.
- Conversation layout may collapse from room list + messages into sequential screens.
- All primary actions must remain keyboard reachable.

## Accessibility and interaction requirements

- WCAG AA contrast at minimum;
- complete keyboard navigation, visible focus, and logical tab order;
- screen-reader labels for live signals, avatars, status-only icons, and unread counts;
- no color-only meaning;
- reduced-motion support;
- live updates should not steal focus or reorder the view unexpectedly;
- destructive actions require explicit confirmation and state consequences;
- loading, empty, stale, permission-denied, reconnecting, and failure states are designed, not left to implementation.

## Realistic prototype data

Use concrete data instead of lorem ipsum:

- Organization: `Nuanzs`
- Workspace: `footfall`
- Projects: `footfall-web`, `footfall-api`, `footfall-infra`
- People: Jorge, María, Daniel
- Agents: Codex, Claude, Kimi
- Repositories: `nuanzs/footfall-web`, `nuanzs/footfall-api`, `nuanzs/footfall-infra`
- Example Intent: `Implementar filtros de tráfico por zona`
- Example scopes: `frontend/src/analytics`, `backend/internal/reports`, `infra/terraform/oci`
- Example branches: `pact/a81c2-filtros-zona`, `main`
- Example Room: `#product-decisions`
- Example Records: an accepted requirement, a disputed decision, an open question, and an active risk.

## Expected deliverables

Produce the work in this order:

1. a concise information-architecture map;
2. the proposed global shell and navigation behavior;
3. high-fidelity desktop screens for:
   - Login;
   - Organization home;
   - Workspace overview;
   - Project Resumen;
   - Project Trabajo en vivo;
   - Project Conversaciones;
   - Project Contexto;
   - Project Código;
   - Acceso y seguridad / Users;
   - User detail;
4. focused states for scope collision, stale agent, GitHub disconnected, empty conversation, invitation success, and device authorization;
5. component inventory and design tokens;
6. responsive behavior for the main shell, tables, drawers, and conversations;
7. a clickable prototype for flows A, B, D, and E;
8. a short rationale explaining how the design separates organization, Workspace, Project, live state, durable knowledge, and settings.

When attached screenshots conflict with this brief, preserve the underlying functionality but redesign the layout. Flag any domain ambiguity instead of silently inventing behavior.
