(() => {
  "use strict";

  const SESSION_TOKEN_KEY = "pact.admin.api-token.v1";
  const OVERVIEW_POLL_MS = 5_000;
  const MAX_VISIBLE_EVENTS = 50;
  const RECONNECT_DELAYS_MS = [1_000, 2_000, 4_000, 8_000, 10_000];

  const state = {
    token: "",
    projects: [],
    selectedProjectID: null,
    overview: null,
    events: [],
    overviewLoading: false,
    overviewLoadingProjectID: null,
    overviewRequestSequence: 0,
    pollTimer: null,
    streamController: null,
    streamGeneration: 0,
    reconnectAttempt: 0,
    lastEventByProject: new Map(),
    renderedEventsKey: "",
    deferredRefresh: null,
  };

  const elements = {
    authView: document.querySelector("#auth-view"),
    authForm: document.querySelector("#auth-form"),
    tokenInput: document.querySelector("#token-input"),
    toggleToken: document.querySelector("#toggle-token"),
    connectButton: document.querySelector("#connect-button"),
    authError: document.querySelector("#auth-error"),
    appShell: document.querySelector("#app-shell"),
    disconnectButton: document.querySelector("#disconnect-button"),
    globalConnection: document.querySelector("#global-connection"),
    projectSearch: document.querySelector("#project-search"),
    projectList: document.querySelector("#project-list"),
    projectListStatus: document.querySelector("#project-list-status"),
    refreshProjects: document.querySelector("#refresh-projects"),
    workspaceEmpty: document.querySelector("#workspace-empty"),
    workspaceContent: document.querySelector("#workspace-content"),
    workspaceError: document.querySelector("#workspace-error"),
    workspaceErrorMessage: document.querySelector("#workspace-error-message"),
    retryOverview: document.querySelector("#retry-overview"),
    refreshOverview: document.querySelector("#refresh-overview"),
    projectName: document.querySelector("#project-name"),
    projectSlug: document.querySelector("#project-slug"),
    projectStatus: document.querySelector("#project-status"),
    projectVersion: document.querySelector("#project-version"),
    projectRevision: document.querySelector("#project-revision"),
    projectID: document.querySelector("#project-id"),
    streamStatus: document.querySelector("#stream-status"),
    eventLiveLabel: document.querySelector("#event-live-label"),
    activityPanel: document.querySelector("#activity-panel"),
    activityTitle: document.querySelector("#activity-title"),
    activityDescription: document.querySelector("#activity-description"),
    activityObservers: document.querySelector("#activity-observers"),
    activityTime: document.querySelector("#activity-time"),
    activitySource: document.querySelector("#activity-source"),
    metricSessions: document.querySelector("#metric-sessions"),
    metricIntents: document.querySelector("#metric-intents"),
    metricIntentsNote: document.querySelector("#metric-intents-note"),
    metricWorkspaces: document.querySelector("#metric-workspaces"),
    metricChangesets: document.querySelector("#metric-changesets"),
    inventoryGrid: document.querySelector("#inventory-grid"),
    activeWorkList: document.querySelector("#active-work-list"),
    activeWorkEmpty: document.querySelector("#active-work-empty"),
    activeWorkCount: document.querySelector("#active-work-count"),
    workboardList: document.querySelector("#workboard-list"),
    workboardEmpty: document.querySelector("#workboard-empty"),
    workboardCount: document.querySelector("#workboard-count"),
    workboardLiveCount: document.querySelector("#workboard-live-count"),
    eventList: document.querySelector("#event-list"),
    eventEmpty: document.querySelector("#event-empty"),
    generatedAt: document.querySelector("#generated-at"),
    toastRegion: document.querySelector("#toast-region"),
  };

  const activityCopy = {
    unobserved: {
      title: "Observador no conectado",
      fallback:
        "Pact no puede confirmar si el código se está modificando hasta que un observador se conecte.",
    },
    idle: {
      title: "Sin cambios recientes",
      fallback: "Hay un observador conectado y no ha detectado cambios recientes.",
    },
    editing: {
      title: "Modificación detectada",
      fallback: "Pact recibió una señal fresca de cambios en el código.",
    },
    recent: {
      title: "Cambios recientes",
      fallback: "Pact detectó actividad de código dentro de la ventana reciente.",
    },
  };

  const reasonCopy = {
    no_connected_observer:
      "Pact no puede confirmar si el código se está modificando hasta que un observador se conecte.",
    observer_connected_no_recent_change:
      "El observador está conectado y no ha detectado cambios recientes.",
    fresh_workspace_diff:
      "Un workspace gestionado acaba de publicar una actualización de su diff.",
    fresh_external_git_change:
      "Pact acaba de detectar un cambio de Git realizado fuera del flujo gestionado.",
    recent_workspace_diff:
      "Un workspace gestionado publicó cambios dentro de la ventana reciente.",
    recent_external_git_change:
      "Se detectó recientemente un cambio de Git fuera del flujo gestionado.",
    recent_changeset:
      "Se creó recientemente un changeset listo para avanzar por el flujo de integración.",
  };

  const projectStatusCopy = {
    initializing: "Inicializando",
    active: "Activo",
    archived: "Archivado",
  };

  const intentStatusCopy = {
    draft: "Borrador",
    active: "En curso",
    blocked: "Bloqueado",
    submitted: "En revisión",
    completed: "Completado",
    cancelled: "Cancelado",
    abandoned: "Abandonado",
  };

  const eventTypeCopy = {
    "pact.intent.started.v1": "Trabajo iniciado",
    "pact.workspace.ready.v1": "Workspace preparado",
    "pact.intent.active.v1": "Trabajo reanudado",
    "pact.intent.blocked.v1": "Trabajo bloqueado",
    "pact.intent.submitted.v1": "Trabajo enviado a revisión",
    "pact.intent.completed.v1": "Trabajo completado",
    "pact.intent.cancelled.v1": "Trabajo cancelado",
    "pact.intent.abandoned.v1": "Trabajo abandonado",
    "pact.workspace.diff_updated.v1": "Código modificado",
    "pact.workspace.head_updated.v1": "Workspace avanzó de revisión",
    "pact.git.external_change_detected.v1": "Cambio externo detectado",
    "pact.session.started.v1": "Agente conectado",
    "pact.session.closed.v1": "Agente desconectado",
  };

  class APIError extends Error {
    constructor(message, status = 0, code = "") {
      super(message);
      this.name = "APIError";
      this.status = status;
      this.code = code;
    }
  }

  function init() {
    bindEvents();
    const token = readSessionToken();
    if (token) {
      connect(token, { automatic: true });
      return;
    }
    showAuth();
  }

  function bindEvents() {
    elements.authForm.addEventListener("submit", (event) => {
      event.preventDefault();
      const token = elements.tokenInput.value.trim();
      if (!token) {
        showAuthError("Introduce un token de acceso.");
        elements.tokenInput.focus();
        return;
      }
      connect(token);
    });

    elements.toggleToken.addEventListener("click", () => {
      const showing = elements.tokenInput.type === "text";
      elements.tokenInput.type = showing ? "password" : "text";
      elements.toggleToken.setAttribute("aria-pressed", String(!showing));
      elements.toggleToken.setAttribute(
        "aria-label",
        showing ? "Mostrar token" : "Ocultar token",
      );
      elements.toggleToken.title = showing ? "Mostrar token" : "Ocultar token";
    });

    elements.disconnectButton.addEventListener("click", disconnect);
    elements.refreshProjects.addEventListener("click", () => refreshProjects());
    elements.refreshOverview.addEventListener("click", () =>
      loadOverview({ announce: true }),
    );
    elements.retryOverview.addEventListener("click", () =>
      loadOverview({ announce: true }),
    );
    elements.projectSearch.addEventListener("input", () => renderProjectList());
    document.addEventListener("visibilitychange", () => {
      if (!document.hidden && state.selectedProjectID) {
        loadOverview({ silent: true });
      }
    });
  }

  async function connect(token, { automatic = false } = {}) {
    setConnectLoading(true);
    hideAuthError();
    setGlobalConnection("connecting", "Conectando");

    try {
      const payload = await requestJSON("/v1/projects", { token });
      const projects = normalizeProjects(payload);
      state.token = token;
      state.projects = projects;
      writeSessionToken(token);
      elements.tokenInput.value = "";
      showApplication();
      renderProjectList();
      setGlobalConnection("connected", "Conectado");

      const retained = projects.find(
        (project) => project.id === state.selectedProjectID,
      );
      if (retained) {
        await selectProject(retained.id);
      } else if (projects.length > 0) {
        await selectProject(projects[0].id);
      } else {
        clearSelection();
      }
    } catch (error) {
      state.token = "";
      removeSessionToken();
      showAuth();
      setGlobalConnection("error", "Sin conectar");
      showAuthError(authErrorMessage(error));
      if (!automatic) {
        elements.tokenInput.focus();
      }
    } finally {
      setConnectLoading(false);
    }
  }

  function disconnect() {
    stopLiveUpdates();
    removeSessionToken();
    state.token = "";
    state.projects = [];
    state.selectedProjectID = null;
    state.overview = null;
    state.events = [];
    state.lastEventByProject.clear();
    elements.tokenInput.value = "";
    showAuth();
    setGlobalConnection("disconnected", "Sin conectar");
    elements.tokenInput.focus();
  }

  function showAuth() {
    elements.authView.hidden = false;
    elements.appShell.hidden = true;
    elements.disconnectButton.hidden = true;
  }

  function showApplication() {
    elements.authView.hidden = true;
    elements.appShell.hidden = false;
    elements.disconnectButton.hidden = false;
  }

  async function refreshProjects() {
    if (!state.token) return;
    setBusy(elements.refreshProjects, true);
    elements.projectListStatus.textContent = "Actualizando…";

    try {
      const payload = await requestJSON("/v1/projects");
      state.projects = normalizeProjects(payload);

      if (
        state.selectedProjectID &&
        !state.projects.some(
          (project) => project.id === state.selectedProjectID,
        )
      ) {
        clearSelection();
      }
      renderProjectList();
      showToast("Lista de proyectos actualizada.");
    } catch (error) {
      handleRequestFailure(error, "No se pudieron actualizar los proyectos.");
      renderProjectList();
    } finally {
      setBusy(elements.refreshProjects, false);
    }
  }

  function normalizeProjects(payload) {
    const data = payload && payload.data;
    if (data && Array.isArray(data.projects)) return data.projects;
    if (Array.isArray(data)) return data;
    return [];
  }

  function renderProjectList({ focusProjectID = null } = {}) {
    const query = elements.projectSearch.value.trim().toLocaleLowerCase("es");
    const projects = state.projects.filter((project) => {
      if (!query) return true;
      return [project.name, project.slug, project.id].some((value) =>
        String(value || "")
          .toLocaleLowerCase("es")
          .includes(query),
      );
    });

    elements.projectList.replaceChildren();

    if (state.projects.length === 0) {
      elements.projectListStatus.textContent = "0 proyectos";
      const empty = document.createElement("p");
      empty.className = "rail-empty";
      empty.textContent =
        "No hay proyectos en esta organización. Crea uno mediante la API para administrarlo aquí.";
      elements.projectList.append(empty);
      return;
    }

    elements.projectListStatus.textContent = query
      ? `${formatInteger(projects.length)} de ${formatInteger(state.projects.length)} proyectos`
      : `${formatInteger(state.projects.length)} ${state.projects.length === 1 ? "proyecto" : "proyectos"}`;

    if (projects.length === 0) {
      const empty = document.createElement("p");
      empty.className = "rail-empty";
      empty.textContent = "Ningún proyecto coincide con el filtro.";
      elements.projectList.append(empty);
      return;
    }

    let focusTarget = null;
    for (const project of projects) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "project-button";
      button.dataset.projectId = project.id;
      button.classList.toggle(
        "is-selected",
        project.id === state.selectedProjectID,
      );
      button.setAttribute(
        "aria-current",
        project.id === state.selectedProjectID ? "page" : "false",
      );

      const dot = document.createElement("span");
      dot.className = "project-status-dot";
      if (Object.hasOwn(projectStatusCopy, project.status)) {
        dot.classList.add(`status-${project.status}`);
      }
      dot.setAttribute("aria-hidden", "true");

      const copy = document.createElement("span");
      const name = document.createElement("strong");
      name.textContent = valueOrDash(project.name);
      const slug = document.createElement("small");
      slug.textContent = [project.slug, projectStatusCopy[project.status]]
        .filter(Boolean)
        .join(" · ") || "—";
      copy.append(name, slug);
      button.append(dot, copy);
      button.addEventListener("click", () =>
        selectProject(project.id, { focusProject: true }),
      );
      if (project.id === focusProjectID) focusTarget = button;
      elements.projectList.append(button);
    }
    if (focusTarget) focusTarget.focus();
  }

  async function selectProject(projectID, { focusProject = false } = {}) {
    const project = state.projects.find((item) => item.id === projectID);
    if (!project) return;

    stopLiveUpdates();
    state.selectedProjectID = projectID;
    state.overview = null;
    state.events = [];
    renderProjectList({
      focusProjectID: focusProject ? projectID : null,
    });
    renderProjectHeader(project);
    resetOverview();
    elements.workspaceEmpty.hidden = true;
    elements.workspaceContent.hidden = false;
    hideWorkspaceError();

    const generation = state.streamGeneration;
    await loadOverview({ silent: true });
    if (
      generation !== state.streamGeneration ||
      state.selectedProjectID !== projectID
    ) {
      return;
    }

    startOverviewPolling();
    runEventStream(projectID, generation);
  }

  function clearSelection() {
    stopLiveUpdates();
    state.selectedProjectID = null;
    state.overview = null;
    state.events = [];
    elements.workspaceContent.hidden = true;
    elements.workspaceEmpty.hidden = false;
    renderProjectList();
  }

  function renderProjectHeader(project) {
    elements.projectName.textContent = valueOrDash(project.name);
    elements.projectSlug.textContent = valueOrDash(project.slug);
    elements.projectID.textContent = valueOrDash(project.id);
    elements.projectID.title = project.id || "";

    const revision = project.canonical_revision;
    elements.projectRevision.textContent = revision
      ? shortenIdentifier(revision, 16)
      : "Sin definir";
    elements.projectRevision.title = revision || "";

    const status = projectStatusCopy[project.status] || project.status;
    elements.projectStatus.hidden = !status;
    elements.projectStatus.textContent = status || "";

    const hasVersion = Number.isFinite(Number(project.version));
    elements.projectVersion.hidden = !hasVersion;
    elements.projectVersion.textContent = hasVersion
      ? `v${formatInteger(Number(project.version))}`
      : "";
  }

  function resetOverview() {
    renderActivity(null);
    renderCounts(null);
    renderActiveWork([]);
    renderWorkItems([]);
    renderEvents([]);
    elements.generatedAt.textContent = "Actualizando estado…";
    setStreamStatus("stopped", "Preparando flujo");
  }

  async function loadOverview({ silent = false, announce = false } = {}) {
    const projectID = state.selectedProjectID;
    if (!projectID || !state.token) return;
    if (
      state.overviewLoading &&
      state.overviewLoadingProjectID === projectID
    ) {
      return;
    }

    const requestSequence = ++state.overviewRequestSequence;
    const requestToken = state.token;
    state.overviewLoading = true;
    state.overviewLoadingProjectID = projectID;
    if (!silent) setBusy(elements.refreshOverview, true);

    try {
      const payload = await requestJSON(
        `/v1/projects/${encodeURIComponent(projectID)}/overview`,
      );
      if (
        requestSequence !== state.overviewRequestSequence ||
        projectID !== state.selectedProjectID ||
        requestToken !== state.token
      ) {
        return;
      }

      const overview = payload && payload.data ? payload.data : payload;
      state.overview = overview || {};
      const project = state.overview.project;
      if (project && project.id === projectID) {
        const index = state.projects.findIndex((item) => item.id === projectID);
        if (index >= 0) state.projects[index] = project;
        renderProjectHeader(project);
      }

      const recentEvents = Array.isArray(state.overview.recent_events)
        ? state.overview.recent_events
        : [];
      state.events = mergeEvents(recentEvents, state.events);
      rememberHighestEvent(projectID, state.events);
      renderOverview();
      hideWorkspaceError();
      if (announce) showToast("Estado del proyecto actualizado.");
    } catch (error) {
      if (
        requestSequence !== state.overviewRequestSequence ||
        projectID !== state.selectedProjectID ||
        requestToken !== state.token
      ) {
        return;
      }
      showWorkspaceError(requestErrorMessage(error));
      handleUnauthorized(error);
    } finally {
      if (requestSequence === state.overviewRequestSequence) {
        state.overviewLoading = false;
        state.overviewLoadingProjectID = null;
        setBusy(elements.refreshOverview, false);
      }
    }
  }

  function renderOverview() {
    const overview = state.overview || {};
    renderActivity(overview.code_activity);
    renderCounts(overview.counts);
    renderActiveWork(
      Array.isArray(overview.active_work) ? overview.active_work : [],
    );
    renderWorkItems(
      Array.isArray(overview.work_items) ? overview.work_items : [],
    );
    renderEvents(state.events);

    if (overview.generated_at) {
      elements.generatedAt.textContent = `Estado generado ${formatDateTime(overview.generated_at)}`;
      elements.generatedAt.title = String(overview.generated_at);
    } else {
      elements.generatedAt.textContent = "Hora de generación no disponible";
      elements.generatedAt.removeAttribute("title");
    }
  }

  function renderActivity(activity) {
    const classes = ["state-unobserved", "state-idle", "state-editing", "state-recent"];
    elements.activityPanel.classList.remove(...classes);

    if (!activity || typeof activity !== "object") {
      elements.activityPanel.classList.add("state-unobserved");
      elements.activityTitle.textContent = "Cargando estado…";
      elements.activityDescription.textContent =
        "Esperando la lectura operativa de Pact Server.";
      elements.activityObservers.textContent = "—";
      elements.activityTime.textContent = "—";
      elements.activitySource.textContent = "—";
      return;
    }

    const knownState = Object.hasOwn(activityCopy, activity.state)
      ? activity.state
      : "unobserved";
    const copy = activityCopy[knownState];
    elements.activityPanel.classList.add(`state-${knownState}`);
    elements.activityTitle.textContent =
      activity.state && !Object.hasOwn(activityCopy, activity.state)
        ? activity.state
        : copy.title;
    elements.activityDescription.textContent =
      reasonCopy[activity.reason] || copy.fallback;
    elements.activityObservers.textContent = isNumber(activity.observer_count)
      ? formatInteger(activity.observer_count)
      : "—";
    elements.activityTime.textContent = activity.observed_at
      ? formatRelativeDate(activity.observed_at)
      : "Sin señales";
    elements.activityTime.title = activity.observed_at || "";
    elements.activitySource.textContent = valueOrDash(activity.source);
    elements.activitySource.title = activity.source || "";
  }

  function renderCounts(counts) {
    const safeCounts = counts && typeof counts === "object" ? counts : {};
    elements.metricSessions.textContent = formatCount(safeCounts.live_sessions);
    elements.metricIntents.textContent = formatCount(safeCounts.active_intents);
    elements.metricWorkspaces.textContent = formatCount(
      safeCounts.live_workspaces,
    );
    elements.metricChangesets.textContent = formatCount(
      safeCounts.pending_changesets,
    );
    elements.metricIntentsNote.textContent = isNumber(
      safeCounts.blocked_intents,
    )
      ? `${formatInteger(safeCounts.blocked_intents)} bloqueadas`
      : "Bloqueos no disponibles";

    for (const target of elements.inventoryGrid.querySelectorAll(
      "[data-count]",
    )) {
      target.textContent = formatCount(safeCounts[target.dataset.count]);
    }
  }

  function renderActiveWork(items) {
    elements.activeWorkList.replaceChildren();
    elements.activeWorkCount.textContent = formatInteger(items.length);
    elements.activeWorkEmpty.hidden = items.length !== 0;

    for (const item of items) {
      const row = document.createElement("article");
      row.className = "work-item";

      const actor = document.createElement("div");
      actor.className = "actor-block";
      const avatar = document.createElement("span");
      avatar.className = "actor-avatar";
      avatar.textContent = initials(item.actor_name || item.actor_id);
      avatar.setAttribute("aria-hidden", "true");
      const actorCopy = document.createElement("div");
      actorCopy.className = "actor-copy";
      const actorName = document.createElement("strong");
      actorName.textContent = valueOrDash(item.actor_name || item.actor_id);
      const actorKind = document.createElement("span");
      actorKind.textContent = [item.actor_kind, item.client_type]
        .filter(Boolean)
        .join(" · ") || "—";
      actorCopy.append(actorName, actorKind);
      actor.append(avatar, actorCopy);

      const context = document.createElement("div");
      context.className = "work-context";
      const contextTitle = document.createElement("strong");
      contextTitle.textContent = valueOrDash(
        item.intent_title || item.workspace_branch || "Conectado, sin trabajo declarado",
      );
      const contextDetail = document.createElement("span");
      contextDetail.textContent = [
        item.intent_status,
        item.workspace_status,
        item.workspace_branch,
      ]
        .filter(Boolean)
        .join(" · ") || valueOrDash(item.node_name);
      context.append(contextTitle, contextDetail);

      const meta = document.createElement("div");
      meta.className = "work-meta";
      const seen = document.createElement("time");
      seen.textContent = item.last_seen_at
        ? formatRelativeDate(item.last_seen_at)
        : "Sin heartbeat";
      if (item.last_seen_at) seen.dateTime = item.last_seen_at;
      const status = document.createElement("span");
      status.className = "status-label";
      status.textContent = valueOrDash(item.session_status);
      meta.append(seen, status);

      row.append(actor, context, meta);
      elements.activeWorkList.append(row);
    }
  }

  function renderWorkItems(items) {
    elements.workboardList.replaceChildren();
    elements.workboardCount.textContent = formatInteger(items.length);
    elements.workboardEmpty.hidden = items.length !== 0;
    const isCurrent = (item) =>
      item.session_live && ["active", "blocked"].includes(item.intent?.status);
    const liveCount = items.filter(isCurrent).length;
    elements.workboardLiveCount.classList.toggle("is-live", liveCount > 0);
    elements.workboardLiveCount.lastChild.textContent =
      `${formatInteger(liveCount)} ${liveCount === 1 ? "en vivo" : "en vivo"}`;

    for (const item of items) {
      const intent = item.intent || {};
      const workspace = item.workspace || null;
      const live = isCurrent(item);
      const card = document.createElement("article");
      card.className = `workboard-item status-${intent.status || "unknown"}`;

      const presence = document.createElement("div");
      presence.className = "workboard-presence";
      const liveDot = document.createElement("span");
      liveDot.className = `presence-dot${live ? " is-live" : ""}`;
      liveDot.setAttribute("aria-hidden", "true");
      const identity = document.createElement("div");
      const person = document.createElement("strong");
      person.textContent = valueOrDash(item.responsible_name);
      const seen = document.createElement("span");
      seen.textContent = live
        ? intent.status === "blocked" ? "Conectado · bloqueado" : "Trabajando ahora"
        : item.session_last_seen_at
          ? `Última señal ${formatRelativeDate(item.session_last_seen_at)}`
          : "Sin sesión activa";
      identity.append(person, seen);
      presence.append(liveDot, identity);

      const main = document.createElement("div");
      main.className = "workboard-main";
      const titleRow = document.createElement("div");
      titleRow.className = "workboard-title-row";
      const title = document.createElement("strong");
      title.textContent = valueOrDash(intent.title);
      const status = document.createElement("span");
      status.className = `intent-status status-${intent.status || "unknown"}`;
      status.textContent = intentStatusCopy[intent.status] || valueOrDash(intent.status);
      titleRow.append(title, status);
      const goal = document.createElement("p");
      goal.textContent = valueOrDash(intent.summary || intent.goal);
      main.append(titleRow, goal);

      const scopeList = document.createElement("div");
      scopeList.className = "scope-list";
      const scopes = Array.isArray(item.scopes) ? item.scopes : [];
      for (const claim of scopes.slice(0, 5)) {
        const chip = document.createElement("code");
        const resource = claim.resource || {};
        chip.className = `scope-chip mode-${claim.mode || "exclusive"}`;
        chip.textContent = `${resource.kind || "scope"}:${resource.locator || "—"}`;
        chip.title = `${claim.mode || "exclusive"} · ${claim.status || "—"}`;
        scopeList.append(chip);
      }
      if (scopes.length > 5) {
        const more = document.createElement("span");
        more.className = "scope-more";
        more.textContent = `+${formatInteger(scopes.length - 5)}`;
        scopeList.append(more);
      }
      main.append(scopeList);

      const location = document.createElement("div");
      location.className = "workboard-location";
      const branch = document.createElement("code");
      branch.textContent = workspace?.git_branch || "Workspace pendiente";
      branch.title = workspace?.git_branch || "";
      const revision = document.createElement("span");
      revision.textContent = intent.base_revision
        ? `base ${shortenIdentifier(intent.base_revision, 10)}`
        : "Base no declarada";
      location.append(branch, revision);

      card.append(presence, main, location);
      elements.workboardList.append(card);
    }
  }

  function renderEvents(events) {
    const renderKey = [
      state.selectedProjectID || "",
      ...events.map((event) => event.id || event.sequence || ""),
    ].join("|");
    if (renderKey === state.renderedEventsKey) return;
    state.renderedEventsKey = renderKey;

    elements.eventList.replaceChildren();
    elements.eventEmpty.hidden = events.length !== 0;

    for (const event of events.slice(0, MAX_VISIBLE_EVENTS)) {
      const item = document.createElement("li");
      item.className = "event-item";

      const sequence = document.createElement("span");
      sequence.className = "event-sequence";
      sequence.textContent = compactSequence(event.sequence);
      sequence.title = event.sequence ? `Secuencia ${event.sequence}` : "";

      const body = document.createElement("div");
      body.className = "event-body";
      const titleRow = document.createElement("div");
      titleRow.className = "event-title-row";
      const type = document.createElement("strong");
      type.className = "event-type";
      type.textContent = eventTypeCopy[event.type] || valueOrDash(event.type);
      type.title = event.type || "";
      const time = document.createElement("time");
      time.className = "event-time";
      time.textContent = event.occurred_at
        ? formatClock(event.occurred_at)
        : "—";
      if (event.occurred_at) {
        time.dateTime = event.occurred_at;
        time.title = formatDateTime(event.occurred_at);
      }
      titleRow.append(type, time);

      const meta = document.createElement("div");
      meta.className = "event-meta";
      appendEventMeta(meta, "Actor", event.actor_name || event.actor_id, event.actor_id);
      appendEventMeta(meta, "Sesión", event.session_id);
      appendEventMeta(meta, "Intent", event.intent_id);

      const data = document.createElement("pre");
      data.className = "event-data";
      data.textContent = formatEventData(event.data);
      body.append(titleRow);
      if (meta.childElementCount > 0) body.append(meta);
      if (data.textContent) body.append(data);

      item.append(sequence, body);
      if (data.textContent) {
        const toggle = document.createElement("button");
        toggle.type = "button";
        toggle.className = "event-toggle";
        toggle.textContent = "+";
        toggle.setAttribute("aria-expanded", "false");
        toggle.setAttribute(
          "aria-label",
          `Mostrar datos del evento ${event.type || ""}`,
        );
        toggle.addEventListener("click", () => {
          const expanded = item.classList.toggle("is-expanded");
          toggle.setAttribute("aria-expanded", String(expanded));
          toggle.setAttribute(
            "aria-label",
            `${expanded ? "Ocultar" : "Mostrar"} datos del evento ${event.type || ""}`,
          );
          toggle.textContent = expanded ? "−" : "+";
        });
        item.append(toggle);
      }
      elements.eventList.append(item);
    }
  }

  function appendEventMeta(container, label, value, fullValue = value) {
    if (!value) return;
    const span = document.createElement("span");
    const code = document.createElement("code");
    code.textContent = value === fullValue ? shortenIdentifier(value, 8) : value;
    code.title = fullValue || value;
    span.append(`${label} `, code);
    container.append(span);
  }

  function startOverviewPolling() {
    clearInterval(state.pollTimer);
    state.pollTimer = window.setInterval(() => {
      if (!document.hidden) loadOverview({ silent: true });
    }, OVERVIEW_POLL_MS);
  }

  function stopLiveUpdates() {
    state.streamGeneration += 1;
    state.reconnectAttempt = 0;
    clearInterval(state.pollTimer);
    state.pollTimer = null;
    clearTimeout(state.deferredRefresh);
    state.deferredRefresh = null;
    if (state.streamController) {
      state.streamController.abort();
      state.streamController = null;
    }
    setStreamStatus("stopped", "Flujo detenido");
  }

  async function runEventStream(projectID, generation) {
    while (
      state.token &&
      state.selectedProjectID === projectID &&
      state.streamGeneration === generation
    ) {
      const controller = new AbortController();
      let connectedAt = 0;
      state.streamController = controller;
      setStreamStatus(
        state.reconnectAttempt === 0 ? "connecting" : "reconnecting",
        state.reconnectAttempt === 0 ? "Conectando flujo" : "Reconectando",
      );

      try {
        const headers = {
          Accept: "text/event-stream",
          Authorization: `Bearer ${state.token}`,
        };
        const lastEventID = state.lastEventByProject.get(projectID);
        if (lastEventID) headers["Last-Event-ID"] = lastEventID;

        const response = await fetch(
          `/v1/projects/${encodeURIComponent(projectID)}/events/stream`,
          {
            method: "GET",
            headers,
            cache: "no-store",
            credentials: "omit",
            signal: controller.signal,
          },
        );

        if (!response.ok) {
          throw await responseError(response);
        }
        const contentType = response.headers.get("content-type") || "";
        if (!contentType.includes("text/event-stream") || !response.body) {
          throw new APIError(
            "El servidor no devolvió un flujo de eventos válido.",
            response.status,
          );
        }

        connectedAt = Date.now();
        setStreamStatus("live", "Eventos en vivo");
        await consumeEventStream(response.body, (message) =>
          receiveStreamEvent(projectID, generation, message),
        );
        if (!controller.signal.aborted) {
          throw new APIError("El flujo de eventos se cerró.");
        }
      } catch (error) {
        if (controller.signal.aborted) return;
        if (connectedAt && Date.now() - connectedAt >= 10_000) {
          state.reconnectAttempt = 0;
        }
        if (handleUnauthorized(error)) return;
        setStreamStatus("reconnecting", "Reconectando");
      } finally {
        if (state.streamController === controller) {
          state.streamController = null;
        }
      }

      if (
        state.selectedProjectID !== projectID ||
        state.streamGeneration !== generation
      ) {
        return;
      }

      const delay =
        RECONNECT_DELAYS_MS[
          Math.min(state.reconnectAttempt, RECONNECT_DELAYS_MS.length - 1)
        ];
      state.reconnectAttempt += 1;
      await abortableDelay(delay, generation);
    }
  }

  async function consumeEventStream(stream, onMessage) {
    const reader = stream.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let message = emptySSEMessage();

    const processLine = (line) => {
      if (line === "") {
        if (message.data.length > 0) {
          onMessage({
            id: message.id,
            event: message.event || "message",
            data: message.data.join("\n"),
          });
        }
        message = emptySSEMessage();
        return;
      }
      if (line.startsWith(":")) return;

      const colon = line.indexOf(":");
      const field = colon === -1 ? line : line.slice(0, colon);
      let value = colon === -1 ? "" : line.slice(colon + 1);
      if (value.startsWith(" ")) value = value.slice(1);

      if (field === "id" && !value.includes("\0")) message.id = value;
      if (field === "event") message.event = value;
      if (field === "data") message.data.push(value);
    };

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        let newline;
        while ((newline = buffer.indexOf("\n")) !== -1) {
          let line = buffer.slice(0, newline);
          buffer = buffer.slice(newline + 1);
          if (line.endsWith("\r")) line = line.slice(0, -1);
          processLine(line);
        }
      }
      buffer += decoder.decode();
      if (buffer) processLine(buffer.endsWith("\r") ? buffer.slice(0, -1) : buffer);
      processLine("");
    } finally {
      reader.releaseLock();
    }
  }

  function receiveStreamEvent(projectID, generation, message) {
    if (
      state.selectedProjectID !== projectID ||
      state.streamGeneration !== generation
    ) {
      return;
    }

    let event;
    try {
      event = JSON.parse(message.data);
    } catch {
      showToast("Pact envió un evento que no pudo interpretarse.", {
        error: true,
      });
      return;
    }

    if (message.id) {
      advanceStreamCursor(projectID, message.id);
    } else if (event.sequence) {
      advanceStreamCursor(projectID, event.sequence);
    }

    state.events = mergeEvents([event], state.events);
    renderEvents(state.events);
    scheduleOverviewRefresh();
  }

  function scheduleOverviewRefresh() {
    clearTimeout(state.deferredRefresh);
    state.deferredRefresh = window.setTimeout(() => {
      state.deferredRefresh = null;
      loadOverview({ silent: true });
    }, 250);
  }

  function mergeEvents(primary, secondary) {
    const merged = [];
    const seen = new Set();
    for (const event of [...primary, ...secondary]) {
      if (!event || typeof event !== "object") continue;
      const key =
        event.id ||
        (event.sequence ? `sequence:${event.sequence}` : JSON.stringify(event));
      if (seen.has(key)) continue;
      seen.add(key);
      merged.push(event);
    }
    merged.sort(compareEventsNewestFirst);
    return merged.slice(0, MAX_VISIBLE_EVENTS);
  }

  function compareEventsNewestFirst(left, right) {
    const leftSequence = numericSequence(left.sequence);
    const rightSequence = numericSequence(right.sequence);
    if (leftSequence !== null && rightSequence !== null) {
      if (leftSequence > rightSequence) return -1;
      if (leftSequence < rightSequence) return 1;
      return 0;
    }
    return (
      safeTimestamp(right.occurred_at) - safeTimestamp(left.occurred_at)
    );
  }

  function rememberHighestEvent(projectID, events) {
    if (state.lastEventByProject.has(projectID)) return;

    let highest = null;
    for (const event of events) {
      const sequence = numericSequence(event.sequence);
      if (sequence !== null && (highest === null || sequence > highest)) {
        highest = sequence;
      }
    }
    if (highest !== null) {
      state.lastEventByProject.set(projectID, highest.toString());
    }
  }

  function advanceStreamCursor(projectID, candidate) {
    const next = numericSequence(candidate);
    if (next === null) return;

    const current = numericSequence(state.lastEventByProject.get(projectID));
    if (current === null || next > current) {
      state.lastEventByProject.set(projectID, next.toString());
    }
  }

  async function requestJSON(path, { token = state.token } = {}) {
    const response = await fetch(path, {
      method: "GET",
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${token}`,
      },
      cache: "no-store",
      credentials: "omit",
    });
    if (!response.ok) throw await responseError(response);
    try {
      return await response.json();
    } catch {
      throw new APIError("Pact Server devolvió una respuesta JSON inválida.");
    }
  }

  async function responseError(response) {
    let body = null;
    try {
      body = await response.json();
    } catch {
      // A response without JSON still carries a useful HTTP status.
    }
    return new APIError(
      (body && (body.detail || body.title)) ||
        `Pact Server respondió con HTTP ${response.status}.`,
      response.status,
      body && body.code,
    );
  }

  function handleRequestFailure(error, fallback) {
    if (handleUnauthorized(error)) return;
    showToast(error instanceof Error ? error.message : fallback, {
      error: true,
    });
  }

  function handleUnauthorized(error) {
    if (!(error instanceof APIError) || error.status !== 401) return false;
    stopLiveUpdates();
    removeSessionToken();
    state.token = "";
    state.projects = [];
    state.selectedProjectID = null;
    showAuth();
    setGlobalConnection("error", "Sesión vencida");
    showAuthError("El token fue rechazado. Introduce uno válido para continuar.");
    elements.tokenInput.focus();
    return true;
  }

  function setGlobalConnection(kind, label) {
    elements.globalConnection.classList.toggle(
      "is-connected",
      kind === "connected",
    );
    elements.globalConnection.classList.toggle("is-error", kind === "error");
    const textNode = [...elements.globalConnection.childNodes]
      .reverse()
      .find((node) => node.nodeType === Node.TEXT_NODE);
    if (textNode) textNode.textContent = ` ${label}`;
  }

  function setStreamStatus(kind, label) {
    elements.streamStatus.classList.remove(
      "is-live",
      "is-reconnecting",
      "is-error",
    );
    if (kind === "live") elements.streamStatus.classList.add("is-live");
    if (kind === "connecting" || kind === "reconnecting") {
      elements.streamStatus.classList.add("is-reconnecting");
    }
    if (kind === "error") elements.streamStatus.classList.add("is-error");

    const textNode = [...elements.streamStatus.childNodes]
      .reverse()
      .find((node) => node.nodeType === Node.TEXT_NODE);
    if (textNode) textNode.textContent = ` ${label}`;

    const live = kind === "live";
    elements.eventLiveLabel.classList.toggle("is-live", live);
    elements.eventLiveLabel.lastChild.textContent = live
      ? " En vivo"
      : kind === "reconnecting" || kind === "connecting"
        ? " Conectando"
        : " En espera";
  }

  function showWorkspaceError(message) {
    elements.workspaceError.hidden = false;
    elements.workspaceErrorMessage.textContent = message;
  }

  function hideWorkspaceError() {
    elements.workspaceError.hidden = true;
    elements.workspaceErrorMessage.textContent = "";
  }

  function showAuthError(message) {
    elements.authError.textContent = message;
    elements.authError.hidden = false;
  }

  function hideAuthError() {
    elements.authError.textContent = "";
    elements.authError.hidden = true;
  }

  function showToast(message, { error = false } = {}) {
    const toast = document.createElement("div");
    toast.className = `toast${error ? " is-error" : ""}`;
    toast.textContent = message;
    elements.toastRegion.append(toast);
    window.setTimeout(() => toast.remove(), 4_000);
  }

  function setConnectLoading(loading) {
    elements.connectButton.disabled = loading;
    const label = elements.connectButton.querySelector("span");
    if (label) {
      label.textContent = loading
        ? "Verificando acceso…"
        : "Entrar al centro de control";
    }
  }

  function setBusy(button, busy) {
    button.disabled = busy;
    button.setAttribute("aria-busy", String(busy));
  }

  function readSessionToken() {
    try {
      return window.sessionStorage.getItem(SESSION_TOKEN_KEY) || "";
    } catch {
      return "";
    }
  }

  function writeSessionToken(token) {
    try {
      window.sessionStorage.setItem(SESSION_TOKEN_KEY, token);
    } catch {
      showToast(
        "El navegador no permitió conservar la sesión; el token seguirá solo en memoria.",
        { error: true },
      );
    }
  }

  function removeSessionToken() {
    try {
      window.sessionStorage.removeItem(SESSION_TOKEN_KEY);
    } catch {
      // The in-memory token is cleared even when storage is unavailable.
    }
  }

  function authErrorMessage(error) {
    if (error instanceof APIError && error.status === 401) {
      return "El token fue rechazado por Pact Server.";
    }
    return requestErrorMessage(error);
  }

  function requestErrorMessage(error) {
    if (error instanceof TypeError) {
      return "No se pudo contactar con Pact Server.";
    }
    if (error instanceof Error && error.message) return error.message;
    return "Ocurrió un error inesperado.";
  }

  function formatCount(value) {
    return isNumber(value) ? formatInteger(value) : "—";
  }

  function formatInteger(value) {
    return new Intl.NumberFormat("es-ES", {
      maximumFractionDigits: 0,
    }).format(Number(value));
  }

  function formatDateTime(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "—";
    return new Intl.DateTimeFormat("es-ES", {
      dateStyle: "medium",
      timeStyle: "medium",
    }).format(date);
  }

  function formatClock(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "—";
    return new Intl.DateTimeFormat("es-ES", {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }).format(date);
  }

  function formatRelativeDate(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "—";
    const seconds = Math.round((date.getTime() - Date.now()) / 1_000);
    const absoluteSeconds = Math.abs(seconds);
    const formatter = new Intl.RelativeTimeFormat("es", { numeric: "auto" });
    if (absoluteSeconds < 60) return formatter.format(seconds, "second");
    if (absoluteSeconds < 3_600) {
      return formatter.format(Math.round(seconds / 60), "minute");
    }
    if (absoluteSeconds < 86_400) {
      return formatter.format(Math.round(seconds / 3_600), "hour");
    }
    return formatter.format(Math.round(seconds / 86_400), "day");
  }

  function formatEventData(value) {
    if (value === null || value === undefined) return "";
    if (typeof value === "string") {
      try {
        return JSON.stringify(JSON.parse(value), null, 2);
      } catch {
        return value;
      }
    }
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }

  function initials(value) {
    const parts = String(value || "")
      .trim()
      .split(/\s+/)
      .filter(Boolean);
    if (parts.length === 0) return "—";
    return parts
      .slice(0, 2)
      .map((part) => part[0])
      .join("")
      .toLocaleUpperCase("es");
  }

  function valueOrDash(value) {
    return value === null || value === undefined || value === ""
      ? "—"
      : String(value);
  }

  function shortenIdentifier(value, length) {
    const text = String(value || "");
    return text.length > length ? `${text.slice(0, length)}…` : text;
  }

  function compactSequence(value) {
    const text = String(value || "—");
    return text.length > 4 ? `…${text.slice(-3)}` : text;
  }

  function isNumber(value) {
    return (
      value !== null &&
      value !== "" &&
      Number.isFinite(Number(value))
    );
  }

  function numericSequence(value) {
    if (value === null || value === undefined || value === "") return null;
    try {
      return BigInt(String(value));
    } catch {
      return null;
    }
  }

  function safeTimestamp(value) {
    const timestamp = new Date(value).getTime();
    return Number.isNaN(timestamp) ? 0 : timestamp;
  }

  function emptySSEMessage() {
    return { id: "", event: "", data: [] };
  }

  function abortableDelay(milliseconds, generation) {
    return new Promise((resolve) => {
      const started = Date.now();
      const check = () => {
        if (
          state.streamGeneration !== generation ||
          Date.now() - started >= milliseconds
        ) {
          resolve();
          return;
        }
        window.setTimeout(check, Math.min(250, milliseconds));
      };
      check();
    });
  }

  init();
})();
