(() => {
  "use strict";

  const SESSION_TOKEN_KEY = "pact.admin.api-token.v1";
  const OVERVIEW_POLL_MS = 5_000;
  const ROOM_POLL_MS = 5_000;
  const MAX_VISIBLE_EVENTS = 50;
  const RECONNECT_DELAYS_MS = [1_000, 2_000, 4_000, 8_000, 10_000];

  const state = {
    token: "",
    workspaces: [],
    projects: [],
    selectedWorkspaceID: null,
    selectedProjectID: null,
    workspaceSection: "overview",
    workspaceRoomDirectories: new Map(),
    workspaceRoomDirectoryLoading: new Set(),
    activeView: "live",
    workspaceContext: null,
    rooms: [],
    roomParticipants: [],
    selectedRoomID: null,
    roomMessages: [],
    roomMentions: [],
    globalRoomMentions: [],
    mentionDialogScope: "workspace",
    roomMentionActorIDs: new Set(),
    replyToMessageID: null,
    roomPollTimer: null,
    roomRequestSequence: 0,
    overview: null,
    projectAccess: null,
    knowledge: null,
    githubStatus: null,
    principal: null,
    projectRepositories: [],
    repositorySyncStates: [],
    availableRepositories: [],
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
    globalRoomMentions: document.querySelector("#global-room-mentions"),
    globalRoomMentionCount: document.querySelector("#global-room-mention-count"),
    projectSearch: document.querySelector("#project-search"),
    projectList: document.querySelector("#project-list"),
    projectListStatus: document.querySelector("#project-list-status"),
    refreshProjects: document.querySelector("#refresh-projects"),
    workspaceOverview: document.querySelector("#workspace-overview"),
    workspaceOverviewName: document.querySelector("#workspace-overview-name"),
    workspaceOverviewStatus: document.querySelector("#workspace-overview-status"),
    workspaceOverviewDescription: document.querySelector("#workspace-overview-description"),
    workspaceProjectCount: document.querySelector("#workspace-project-count"),
    workspaceActiveProjectCount: document.querySelector("#workspace-active-project-count"),
    workspaceRepositoryCount: document.querySelector("#workspace-repository-count"),
    workspaceContextCount: document.querySelector("#workspace-context-count"),
    workspaceProjectListCount: document.querySelector("#workspace-project-list-count"),
    workspaceProjectList: document.querySelector("#workspace-project-list"),
    workspaceContextPreview: document.querySelector("#workspace-context-preview"),
    workspaceConversations: document.querySelector("#workspace-conversations"),
    workspaceRoomCount: document.querySelector("#workspace-room-count"),
    workspaceRoomList: document.querySelector("#workspace-room-list"),
    selectedRoomName: document.querySelector("#selected-room-name"),
    selectedRoomDescription: document.querySelector("#selected-room-description"),
    roomLiveStatus: document.querySelector("#room-live-status"),
    roomMessageList: document.querySelector("#room-message-list"),
    roomMessageEmpty: document.querySelector("#room-message-empty"),
    roomMessageForm: document.querySelector("#room-message-form"),
    roomMessageInput: document.querySelector("#room-message-input"),
    sendRoomMessage: document.querySelector("#send-room-message"),
    roomMentionMenu: document.querySelector("#room-mention-menu"),
    roomReplyPreview: document.querySelector("#room-reply-preview"),
    roomReplyCopy: document.querySelector("#room-reply-copy"),
    cancelRoomReply: document.querySelector("#cancel-room-reply"),
    openCreateRoom: document.querySelector("#open-create-room"),
    createRoomDialog: document.querySelector("#create-room-dialog"),
    createRoomForm: document.querySelector("#create-room-form"),
    createRoomName: document.querySelector("#create-room-name"),
    createRoomDescription: document.querySelector("#create-room-description"),
    createRoomSubmit: document.querySelector("#create-room-submit"),
    cancelCreateRoom: document.querySelector("#cancel-create-room"),
    openRoomMentions: document.querySelector("#open-room-mentions"),
    roomMentionCount: document.querySelector("#room-mention-count"),
    roomMentionsDialog: document.querySelector("#room-mentions-dialog"),
    roomMentionList: document.querySelector("#room-mention-list"),
    roomMentionsKicker: document.querySelector("#room-mentions-kicker"),
    roomMentionsTitle: document.querySelector("#room-mentions-title"),
    workspaceEmpty: document.querySelector("#workspace-empty"),
    workspaceContent: document.querySelector("#workspace-content"),
    workspaceError: document.querySelector("#workspace-error"),
    workspaceErrorMessage: document.querySelector("#workspace-error-message"),
    retryOverview: document.querySelector("#retry-overview"),
    refreshOverview: document.querySelector("#refresh-overview"),
    openProjectSettings: document.querySelector("#open-project-settings"),
    projectName: document.querySelector("#project-name"),
    workspaceName: document.querySelector("#workspace-name"),
    projectSlug: document.querySelector("#project-slug"),
    projectStatus: document.querySelector("#project-status"),
    projectVersion: document.querySelector("#project-version"),
    projectRevision: document.querySelector("#project-revision"),
    projectID: document.querySelector("#project-id"),
    projectTabs: document.querySelector("#project-tabs"),
    dashboardPanels: [...document.querySelectorAll("[data-dashboard-sections]")],
    dashboardViewKicker: document.querySelector("#dashboard-view-kicker"),
    dashboardViewTitle: document.querySelector("#dashboard-view-title"),
    dashboardViewDescription: document.querySelector("#dashboard-view-description"),
    tabLiveCount: document.querySelector("#tab-live-count"),
    tabWorkCount: document.querySelector("#tab-work-count"),
    tabKnowledgeCount: document.querySelector("#tab-knowledge-count"),
    tabAccessCount: document.querySelector("#tab-access-count"),
    tabRepositoryCount: document.querySelector("#tab-repository-count"),
    tabActivityCount: document.querySelector("#tab-activity-count"),
    streamStatus: document.querySelector("#stream-status"),
    eventLiveLabel: document.querySelector("#event-live-label"),
    attentionPanel: document.querySelector("#attention-panel"),
    attentionDescription: document.querySelector("#attention-description"),
    attentionStatus: document.querySelector("#attention-status"),
    attentionStats: document.querySelector("#attention-stats"),
    attentionList: document.querySelector("#attention-list"),
    activityPanel: document.querySelector("#activity-panel"),
    activityTitle: document.querySelector("#activity-title"),
    activityDescription: document.querySelector("#activity-description"),
    activityObservers: document.querySelector("#activity-observers"),
    activityTime: document.querySelector("#activity-time"),
    activitySource: document.querySelector("#activity-source"),
    repositorySyncPanel: document.querySelector("#repository-sync-panel"),
    repositorySyncStatus: document.querySelector("#repository-sync-status"),
    repositorySyncDescription: document.querySelector("#repository-sync-description"),
    repositorySyncName: document.querySelector("#repository-sync-name"),
    repositorySyncBranch: document.querySelector("#repository-sync-branch"),
    repositorySyncRevision: document.querySelector("#repository-sync-revision"),
    repositorySyncTime: document.querySelector("#repository-sync-time"),
    syncRepository: document.querySelector("#sync-repository"),
    githubConnectionCard: document.querySelector(".github-connection-card"),
    githubConnectionStatus: document.querySelector("#github-connection-status"),
    githubConnectionDescription: document.querySelector("#github-connection-description"),
    connectGitHub: document.querySelector("#connect-github"),
    attachRepositoryForm: document.querySelector("#attach-repository-form"),
    availableRepository: document.querySelector("#available-repository"),
    repositoryPurpose: document.querySelector("#repository-purpose"),
    repositoryRequired: document.querySelector("#repository-required"),
    repositoryPrimary: document.querySelector("#repository-primary"),
    attachRepository: document.querySelector("#attach-repository"),
    repositoryAttachHint: document.querySelector("#repository-attach-hint"),
    projectRepositoryCount: document.querySelector("#project-repository-count"),
    projectRepositoryList: document.querySelector("#project-repository-list"),
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
    knowledgeGrid: document.querySelector("#knowledge-grid"),
    knowledgeEmpty: document.querySelector("#knowledge-empty"),
    knowledgeCount: document.querySelector("#knowledge-count"),
    handoffList: document.querySelector("#handoff-list"),
    handoffEmpty: document.querySelector("#handoff-empty"),
    handoffCount: document.querySelector("#handoff-count"),
    eventList: document.querySelector("#event-list"),
    eventEmpty: document.querySelector("#event-empty"),
    accessLiveStatus: document.querySelector("#access-live-status"),
    accessMemberCount: document.querySelector("#access-member-count"),
    accessAgentCount: document.querySelector("#access-agent-count"),
    accessConnectedCount: document.querySelector("#access-connected-count"),
    accessMemberListCount: document.querySelector("#access-member-list-count"),
    accessAgentListCount: document.querySelector("#access-agent-list-count"),
    accessMemberList: document.querySelector("#access-member-list"),
    accessAgentList: document.querySelector("#access-agent-list"),
    accessMemberEmpty: document.querySelector("#access-member-empty"),
    accessAgentEmpty: document.querySelector("#access-agent-empty"),
    settingsWorkspace: document.querySelector("#settings-workspace"),
    settingsRole: document.querySelector("#settings-role"),
    settingsProjectID: document.querySelector("#settings-project-id"),
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
      "Un worktree gestionado acaba de publicar una actualización de su diff.",
    fresh_external_git_change:
      "Pact acaba de detectar un cambio de Git realizado fuera del flujo gestionado.",
    recent_workspace_diff:
      "Un worktree gestionado publicó cambios dentro de la ventana reciente.",
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

  const organizationRoleCopy = {
    owner: "Propietario",
    admin: "Administrador",
    member: "Miembro",
    viewer: "Observador",
  };

  const projectRoleCopy = {
    owner: "Propietario",
    maintainer: "Responsable",
    contributor: "Colaborador",
    viewer: "Observador",
  };

  const accessSourceCopy = {
    organization: "Acceso global de la organización",
    project: "Membresía directa del proyecto",
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
    "pact.project.created.v1": "Proyecto creado",
    "pact.intent.started.v1": "Trabajo iniciado",
    "pact.workspace.ready.v1": "Worktree preparado",
    "pact.intent.active.v1": "Trabajo reanudado",
    "pact.intent.blocked.v1": "Trabajo bloqueado",
    "pact.intent.submitted.v1": "Trabajo enviado a revisión",
    "pact.intent.completed.v1": "Trabajo completado",
    "pact.intent.cancelled.v1": "Trabajo cancelado",
    "pact.intent.abandoned.v1": "Trabajo abandonado",
    "pact.workspace.diff_updated.v1": "Código modificado",
    "pact.workspace.head_updated.v1": "Worktree avanzó de revisión",
    "pact.git.external_change_detected.v1": "Cambio externo detectado",
    "pact.session.started.v1": "Agente conectado",
    "pact.session.closed.v1": "Agente desconectado",
    "pact.repository.canonical_synced.v1": "Repositorio canónico verificado",
    "pact.repository.sync_failed.v1": "Falló la verificación de GitHub",
    "pact.project.repository_attached.v1": "Repositorio vinculado",
    "pact.repository-observation.v1": "Código observado",
    "pact.changeset.created.v1": "Cambios preparados",
    "pact.handoff.offered.v1": "Relevo ofrecido",
    "pact.handoff.accepted.v1": "Relevo aceptado",
    "pact.handoff.expired.v1": "Relevo vencido",
    "pact.context.compiled.v1": "Contexto compilado",
    "pact.knowledge.record.proposed.v1": "Conocimiento propuesto",
    "pact.knowledge.resource.added.v1": "Fuente añadida",
  };

  const dashboardViews = {
    live: {
      kicker: "OPERACIÓN · AHORA",
      title: "Quién trabaja y dónde",
      description: "Presencia, alcances y señales de código actualizadas en tiempo real.",
    },
    overview: {
      kicker: "SALUD DEL PROYECTO",
      title: "Resumen operativo",
      description: "Indicadores, fuente canónica e inventario para entender el estado general.",
    },
    work: {
      kicker: "COORDINACIÓN",
      title: "Trabajo e intercambios",
      description: "Intenciones, alcances, bloqueos y handoffs entre colaboradores.",
    },
    knowledge: {
      kicker: "MEMORIA COMPARTIDA",
      title: "Conocimiento del workspace",
      description: "Decisiones, requisitos, restricciones, riesgos y fuentes durables.",
    },
    access: {
      kicker: "IDENTIDAD Y AUTORIZACIÓN",
      title: "Quién puede entrar",
      description: "Personas autorizadas, agentes patrocinados y conexiones vigentes del proyecto.",
    },
    repositories: {
      kicker: "CÓDIGO Y PROVEEDORES",
      title: "Repositorios del proyecto",
      description: "Fuentes autorizadas, propósito y revisión verificable de cada componente.",
    },
    activity: {
      kicker: "AUDITORÍA · EN VIVO",
      title: "Actividad del proyecto",
      description: "Un registro humano y durable de lo que ocurre en PACT.",
    },
    settings: {
      kicker: "ADMINISTRACIÓN",
      title: "Configuración",
      description: "Integraciones, acceso y pertenencia del proyecto dentro de la organización.",
    },
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
    elements.globalRoomMentions.addEventListener("click", openGlobalRoomMentionsDialog);
    elements.refreshProjects.addEventListener("click", () => refreshProjects());
    elements.refreshOverview.addEventListener("click", () =>
      loadOverview({ announce: true }),
    );
    elements.workspaceName.addEventListener("click", () => {
      if (state.selectedWorkspaceID) selectWorkspace(state.selectedWorkspaceID);
    });
    elements.openProjectSettings.addEventListener("click", () =>
      setDashboardView("settings"),
    );
    elements.syncRepository.addEventListener("click", syncCanonicalRepository);
    elements.connectGitHub.addEventListener("click", connectGitHub);
    elements.attachRepositoryForm.addEventListener("submit", attachRepository);
    elements.retryOverview.addEventListener("click", () =>
      loadOverview({ announce: true }),
    );
    elements.openCreateRoom.addEventListener("click", openCreateRoomDialog);
    elements.createRoomForm.addEventListener("submit", createRoom);
    elements.cancelCreateRoom.addEventListener("click", closeCreateRoomDialog);
    elements.createRoomDialog
      .querySelector("[data-close-room-dialog]")
      .addEventListener("click", closeCreateRoomDialog);
    elements.roomMessageForm.addEventListener("submit", postRoomMessage);
    elements.roomMessageInput.addEventListener("input", renderRoomMentionMenu);
    elements.roomMessageInput.addEventListener("keydown", handleRoomComposerKeydown);
    elements.cancelRoomReply.addEventListener("click", clearRoomReply);
    elements.openRoomMentions.addEventListener("click", openRoomMentionsDialog);
    elements.roomMentionsDialog
      .querySelector("[data-close-mentions-dialog]")
      .addEventListener("click", () => elements.roomMentionsDialog.close());
    elements.projectSearch.addEventListener("input", () => renderProjectList());
    for (const tab of elements.projectTabs.querySelectorAll("[data-dashboard-view]")) {
      tab.addEventListener("click", () => setDashboardView(tab.dataset.dashboardView));
      tab.addEventListener("keydown", handleDashboardTabKey);
    }
    document.addEventListener("visibilitychange", () => {
      if (!document.hidden && state.selectedProjectID) {
        loadOverview({ silent: true });
      }
      if (!document.hidden && state.selectedWorkspaceID && state.selectedRoomID) {
        loadRoomMessages({ silent: true });
        loadRoomMentions({ silent: true });
      }
      if (!document.hidden && state.token) loadGlobalRoomMentions({ silent: true });
    });
  }

  async function connect(token, { automatic = false } = {}) {
    setConnectLoading(true);
    hideAuthError();
    setGlobalConnection("connecting", "Conectando");

    try {
      const [projectPayload, workspacePayload, githubPayload, principalPayload] = await Promise.all([
        requestJSON("/v1/projects", { token }),
        requestJSON("/v1/workspaces", { token }),
        requestJSON("/v1/integrations/github", { token }),
        requestJSON("/v1/me", { token }),
      ]);
      const projects = normalizeProjects(projectPayload);
      state.token = token;
      state.projects = projects;
      state.workspaces = normalizeWorkspaces(workspacePayload);
      state.githubStatus = githubPayload?.data || githubPayload || null;
      state.principal = principalPayload?.data || principalPayload || null;
      writeSessionToken(token);
      elements.tokenInput.value = "";
      showApplication();
      renderProjectList();
      setGlobalConnection("connected", "Conectado");
      renderGitHubStatus();
      announceGitHubCallback();
      await loadGlobalRoomMentions({ silent: true });
      if (!state.token) return;

      const retained = projects.find(
        (project) => project.id === state.selectedProjectID,
      );
      if (retained) {
        await selectProject(retained.id);
      } else if (state.workspaces.length > 0) {
        await selectWorkspace(state.workspaces[0].id);
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
    state.workspaces = [];
    state.projects = [];
    state.selectedWorkspaceID = null;
    state.selectedProjectID = null;
    state.workspaceSection = "overview";
    state.workspaceRoomDirectories.clear();
    state.workspaceRoomDirectoryLoading.clear();
    state.workspaceContext = null;
    resetWorkspaceRooms();
    state.globalRoomMentions = [];
    renderGlobalRoomMentionCount();
    state.overview = null;
    state.projectAccess = null;
    state.knowledge = null;
    state.githubStatus = null;
    state.principal = null;
    state.projectRepositories = [];
    state.repositorySyncStates = [];
    state.availableRepositories = [];
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
    elements.globalRoomMentions.hidden = true;
  }

  function showApplication() {
    elements.authView.hidden = true;
    elements.appShell.hidden = false;
    elements.disconnectButton.hidden = false;
    elements.globalRoomMentions.hidden = false;
  }

  async function refreshProjects() {
    if (!state.token) return;
    setBusy(elements.refreshProjects, true);
    elements.projectListStatus.textContent = "Actualizando…";

    try {
      const [projectPayload, workspacePayload, githubPayload] = await Promise.all([
        requestJSON("/v1/projects"),
        requestJSON("/v1/workspaces"),
        requestJSON("/v1/integrations/github"),
      ]);
      state.projects = normalizeProjects(projectPayload);
      state.workspaces = normalizeWorkspaces(workspacePayload);
      state.githubStatus = githubPayload?.data || githubPayload || null;
      const currentWorkspaceIDs = new Set(state.workspaces.map((workspace) => workspace.id));
      for (const workspaceID of state.workspaceRoomDirectories.keys()) {
        if (!currentWorkspaceIDs.has(workspaceID)) {
          state.workspaceRoomDirectories.delete(workspaceID);
        }
      }
      renderGitHubStatus();
      await loadGlobalRoomMentions({ silent: true });
      if (!state.token) return;

      if (
        state.selectedProjectID &&
        !state.projects.some(
          (project) => project.id === state.selectedProjectID,
        )
      ) {
        clearSelection();
      } else if (
        state.selectedWorkspaceID &&
        !state.workspaces.some((workspace) => workspace.id === state.selectedWorkspaceID)
      ) {
        clearSelection();
      } else if (state.selectedWorkspaceID && !state.selectedProjectID) {
        const workspace = state.workspaces.find(
          (item) => item.id === state.selectedWorkspaceID,
        );
        if (workspace) renderWorkspaceOverview(workspace, state.workspaceContext);
      }
      renderProjectList();
      if (state.selectedWorkspaceID) {
        loadWorkspaceRoomDirectory(state.selectedWorkspaceID, { silent: true });
      }
      showToast("Workspaces y proyectos actualizados.");
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

  function normalizeWorkspaces(payload) {
    const data = payload && payload.data;
    if (data && Array.isArray(data.workspaces)) {
      return data.workspaces.filter(
        (workspace) => workspace.status !== "archived" || (workspace.projects || []).length > 0,
      );
    }
    if (Array.isArray(data)) {
      return data.filter(
        (workspace) => workspace.status !== "archived" || (workspace.projects || []).length > 0,
      );
    }
    return [];
  }

  function renderProjectList({ focusProjectID = null } = {}) {
    const query = elements.projectSearch.value.trim().toLocaleLowerCase("es");
    const projectByID = new Map(state.projects.map((project) => [project.id, project]));
    const groups = state.workspaces
      .map((workspace) => {
        const workspaceMatches = [workspace.name, workspace.slug, workspace.id].some(
          (value) => String(value || "").toLocaleLowerCase("es").includes(query),
        );
        const projects = (workspace.projects || [])
          .map((project) => projectByID.get(project.id) || project)
          .filter((project) => {
            if (!query || workspaceMatches) return true;
            return [project.name, project.slug, project.id].some((value) =>
              String(value || "").toLocaleLowerCase("es").includes(query),
            );
          });
        return { workspace, projects };
      })
      .filter(({ workspace, projects }) => {
        if (projects.length > 0) return true;
        if (!query) return true;
        return [workspace.name, workspace.slug, workspace.id].some((value) =>
          String(value || "").toLocaleLowerCase("es").includes(query),
        );
      });
    const visibleProjectCount = groups.reduce((count, group) => count + group.projects.length, 0);

    elements.projectList.replaceChildren();

    if (state.workspaces.length === 0) {
      elements.projectListStatus.textContent = "0 workspaces · 0 proyectos";
      const empty = document.createElement("p");
      empty.className = "rail-empty";
      empty.textContent =
        "No hay proyectos en esta organización. Pact creará un workspace al inicializar el primero.";
      elements.projectList.append(empty);
      return;
    }

    elements.projectListStatus.textContent = query
      ? `${formatInteger(visibleProjectCount)} de ${formatInteger(state.projects.length)} proyectos`
      : `${formatInteger(state.workspaces.length)} ${state.workspaces.length === 1 ? "workspace" : "workspaces"} · ${formatInteger(state.projects.length)} proyectos`;

    if (groups.length === 0) {
      const empty = document.createElement("p");
      empty.className = "rail-empty";
      empty.textContent = "Ningún workspace o proyecto coincide con el filtro.";
      elements.projectList.append(empty);
      return;
    }

    let focusTarget = null;
    for (const group of groups) {
      const groupElement = document.createElement("section");
      groupElement.className = "workspace-group";
      const isCurrentWorkspace = group.workspace.id === state.selectedWorkspaceID;
      groupElement.classList.toggle("is-current", isCurrentWorkspace);

      const heading = document.createElement("button");
      heading.type = "button";
      heading.className = "workspace-group-heading";
      heading.classList.toggle("is-current", isCurrentWorkspace);
      heading.setAttribute("aria-expanded", String(isCurrentWorkspace));
      heading.setAttribute("aria-label", `Abrir workspace ${group.workspace.name}`);

      const workspaceIcon = document.createElement("span");
      workspaceIcon.className = "workspace-heading-icon";
      workspaceIcon.textContent = "W";
      workspaceIcon.setAttribute("aria-hidden", "true");

      const workspaceCopy = document.createElement("span");
      workspaceCopy.className = "workspace-heading-copy";
      const workspaceKind = document.createElement("small");
      workspaceKind.textContent = "Workspace";
      const workspaceName = document.createElement("strong");
      workspaceName.textContent = valueOrDash(group.workspace.name);
      workspaceCopy.append(workspaceKind, workspaceName);

      const workspaceMeta = document.createElement("span");
      workspaceMeta.className = "workspace-heading-meta";
      const workspaceCount = document.createElement("span");
      workspaceCount.textContent = String(group.projects.length);
      workspaceCount.title = `${group.projects.length} proyectos`;
      const chevron = document.createElement("span");
      chevron.textContent = "›";
      chevron.setAttribute("aria-hidden", "true");
      workspaceMeta.append(workspaceCount, chevron);
      heading.append(workspaceIcon, workspaceCopy, workspaceMeta);
      heading.addEventListener("click", () =>
        selectWorkspace(group.workspace.id, { section: "overview" }),
      );
      groupElement.append(heading);

      if (isCurrentWorkspace) {
        const workspaceNavigation = document.createElement("div");
        workspaceNavigation.className = "workspace-navigation";
        workspaceNavigation.append(
          createWorkspaceNavigationButton({
            icon: "⌂",
            label: "Resumen del workspace",
            selected: !state.selectedProjectID && state.workspaceSection === "overview",
            onClick: () => selectWorkspace(group.workspace.id, { section: "overview" }),
          }),
        );

        const directory = state.workspaceRoomDirectories.get(group.workspace.id);
        const pendingMentions = state.globalRoomMentions.filter(
          (mention) => mention.workspace_id === group.workspace.id,
        );
        workspaceNavigation.append(
          createWorkspaceNavigationButton({
            icon: "#",
            label: "Conversaciones",
            count: directory ? directory.length : null,
            badge: pendingMentions.length,
            selected: !state.selectedProjectID && state.workspaceSection === "conversations",
            onClick: () => openWorkspaceConversations(group.workspace.id),
          }),
        );

        const roomList = document.createElement("div");
        roomList.className = "rail-room-list";
        if (!directory) {
          const loading = document.createElement("p");
          loading.className = "rail-room-state";
          loading.textContent = "Cargando conversaciones…";
          roomList.append(loading);
        } else if (directory.length === 0) {
          const empty = document.createElement("p");
          empty.className = "rail-room-state";
          empty.textContent = "Sin conversaciones";
          roomList.append(empty);
        } else {
          const pendingRoomIDs = new Set(pendingMentions.map((mention) => mention.room_id));
          for (const room of directory.slice(0, 5)) {
            const roomButton = document.createElement("button");
            roomButton.type = "button";
            roomButton.className = "rail-room-button";
            roomButton.classList.toggle(
              "is-selected",
              !state.selectedProjectID &&
                state.workspaceSection === "conversations" &&
                room.id === state.selectedRoomID,
            );
            roomButton.setAttribute(
              "aria-current",
              !state.selectedProjectID &&
                state.workspaceSection === "conversations" &&
                room.id === state.selectedRoomID
                ? "page"
                : "false",
            );
            const hash = document.createElement("span");
            hash.textContent = "#";
            hash.setAttribute("aria-hidden", "true");
            const roomName = document.createElement("span");
            roomName.textContent = room.name;
            roomButton.append(hash, roomName);
            if (pendingRoomIDs.has(room.id)) {
              const unread = document.createElement("small");
              unread.textContent = "●";
              unread.setAttribute("aria-label", "Tiene menciones pendientes");
              roomButton.append(unread);
            }
            roomButton.addEventListener("click", () =>
              openWorkspaceConversation(group.workspace.id, room.id),
            );
            roomList.append(roomButton);
          }
          if (directory.length > 5) {
            const allRooms = document.createElement("button");
            allRooms.type = "button";
            allRooms.className = "rail-room-more";
            allRooms.textContent = `Ver todas (${directory.length})`;
            allRooms.addEventListener("click", () => openWorkspaceConversations(group.workspace.id));
            roomList.append(allRooms);
          }
        }
        workspaceNavigation.append(roomList);
        groupElement.append(workspaceNavigation);

        const projectsLabel = document.createElement("p");
        projectsLabel.className = "rail-section-label";
        projectsLabel.textContent = "Proyectos";
        groupElement.append(projectsLabel);
      }

      for (const project of (isCurrentWorkspace || query ? group.projects : [])) {
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
        groupElement.append(button);
      }
      elements.projectList.append(groupElement);
    }
    if (focusTarget) focusTarget.focus();
  }

  function createWorkspaceNavigationButton({ icon, label, count = null, badge = 0, selected, onClick }) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "workspace-navigation-button";
    button.classList.toggle("is-selected", selected);
    button.setAttribute("aria-current", selected ? "page" : "false");
    const symbol = document.createElement("span");
    symbol.textContent = icon;
    symbol.setAttribute("aria-hidden", "true");
    const copy = document.createElement("strong");
    copy.textContent = label;
    button.append(symbol, copy);
    if (count !== null) {
      const total = document.createElement("small");
      total.textContent = String(count);
      total.title = `${count} conversaciones`;
      button.append(total);
    }
    if (badge > 0) {
      const unread = document.createElement("em");
      unread.textContent = badge > 99 ? "99+" : String(badge);
      unread.setAttribute("aria-label", `${badge} menciones pendientes`);
      button.append(unread);
    }
    button.addEventListener("click", onClick);
    return button;
  }

  async function selectProject(projectID, { focusProject = false } = {}) {
    const project = state.projects.find((item) => item.id === projectID);
    if (!project) return;
    const workspace = workspaceForProject(projectID);

    stopLiveUpdates();
    state.selectedWorkspaceID = workspace?.id || null;
    state.selectedProjectID = projectID;
    state.workspaceSection = "overview";
    state.workspaceContext = null;
    resetWorkspaceRooms();
    state.overview = null;
    state.projectAccess = null;
    state.knowledge = null;
    state.projectRepositories = [];
    state.repositorySyncStates = [];
    state.availableRepositories = [];
    state.events = [];
    renderProjectList({
      focusProjectID: focusProject ? projectID : null,
    });
    renderProjectHeader(project);
    resetOverview();
    elements.workspaceOverview.hidden = true;
    elements.workspaceEmpty.hidden = true;
    elements.workspaceContent.hidden = false;
    setDashboardView("live");
    hideWorkspaceError();
    if (workspace) loadWorkspaceRoomDirectory(workspace.id, { silent: true });

    const generation = state.streamGeneration;
    await loadOverview({ silent: true });
    await loadProjectRepositories({ silent: true });
    if (
      generation !== state.streamGeneration ||
      state.selectedProjectID !== projectID
    ) {
      return;
    }

    startOverviewPolling();
    runEventStream(projectID, generation);
  }

  async function selectWorkspace(
    workspaceID,
    { section = "overview", roomID = null, focusMessageID = null } = {},
  ) {
    const workspace = state.workspaces.find((item) => item.id === workspaceID);
    if (!workspace) return;

    stopLiveUpdates();
    state.selectedWorkspaceID = workspaceID;
    state.selectedProjectID = null;
    state.workspaceSection = section;
    state.workspaceContext = null;
    resetWorkspaceRooms();
    state.overview = null;
    state.projectAccess = null;
    state.knowledge = null;
    state.projectRepositories = [];
    state.repositorySyncStates = [];
    state.availableRepositories = [];
    state.events = [];
    renderProjectList();
    elements.workspaceContent.hidden = true;
    elements.workspaceEmpty.hidden = true;
    elements.workspaceOverview.hidden = false;
    renderWorkspaceOverview(workspace, null, { loading: true });

    try {
      const [contextPayload, roomsPayload, participantsPayload, mentionsPayload] = await Promise.all([
        requestJSON(`/v1/workspaces/${encodeURIComponent(workspaceID)}/context`),
        requestJSON(`/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms`),
        requestJSON(`/v1/workspaces/${encodeURIComponent(workspaceID)}/participants`),
        requestJSON(`/v1/me/room-mentions?workspace_id=${encodeURIComponent(workspaceID)}&status=pending&limit=100`),
      ]);
      if (state.selectedWorkspaceID !== workspaceID || state.selectedProjectID) return;
      state.workspaceContext = contextPayload?.data || contextPayload || {};
      state.rooms = roomsPayload?.data?.rooms || [];
      state.workspaceRoomDirectories.set(workspaceID, state.rooms);
      state.roomParticipants = participantsPayload?.data?.participants || [];
      state.roomMentions = (mentionsPayload?.data?.mentions || []).filter(
        (mention) => mention.workspace_id === workspaceID,
      );
      renderWorkspaceOverview(workspace, state.workspaceContext);
      renderRoomDirectory();
      renderRoomMentionInbox();
      renderProjectList();
      const preferredRoom =
        state.rooms.find((room) => room.id === roomID) ||
        state.rooms.find((room) => room.managed_default) ||
        state.rooms[0] ||
        null;
      if (preferredRoom) {
        await selectRoom(preferredRoom.id, { focusMessageID });
        if (state.selectedWorkspaceID !== workspaceID || state.selectedProjectID) return;
        state.workspaceSection = section;
        renderProjectList();
        if (state.selectedWorkspaceID === workspaceID && !state.selectedProjectID) {
          startRoomPolling();
        }
      } else {
        renderRoomConversation();
      }
      focusWorkspaceSection(section, { focusComposer: Boolean(roomID || focusMessageID) });
    } catch (error) {
      if (handleUnauthorized(error)) return;
      if (state.selectedWorkspaceID !== workspaceID || state.selectedProjectID) return;
      renderWorkspaceOverview(workspace, null, { failed: true });
    }
  }

  async function openWorkspaceConversations(workspaceID) {
    const directory = state.workspaceRoomDirectories.get(workspaceID) || [];
    const preferredRoom = directory.find((room) => room.managed_default) || directory[0] || null;
    await selectWorkspace(workspaceID, {
      section: "conversations",
      roomID: preferredRoom?.id || null,
    });
  }

  async function openWorkspaceConversation(workspaceID, roomID, { focusMessageID = null } = {}) {
    await selectWorkspace(workspaceID, {
      section: "conversations",
      roomID,
      focusMessageID,
    });
  }

  function focusWorkspaceSection(section, { focusComposer = false } = {}) {
    const target = section === "conversations"
      ? elements.workspaceConversations
      : elements.workspaceOverview;
    window.requestAnimationFrame(() => {
      target.scrollIntoView({ behavior: "smooth", block: "start" });
      if (section === "conversations") target.focus({ preventScroll: true });
      if (focusComposer && !elements.roomMessageInput.disabled) {
        elements.roomMessageInput.focus({ preventScroll: true });
      }
    });
  }

  async function loadWorkspaceRoomDirectory(workspaceID, { silent = false, force = false } = {}) {
    if (!workspaceID || state.workspaceRoomDirectoryLoading.has(workspaceID)) return;
    if (!force && state.workspaceRoomDirectories.has(workspaceID)) {
      renderProjectList();
      return;
    }
    state.workspaceRoomDirectoryLoading.add(workspaceID);
    try {
      const payload = await requestJSON(
        `/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms`,
      );
      state.workspaceRoomDirectories.set(workspaceID, payload?.data?.rooms || []);
      if (state.selectedWorkspaceID === workspaceID) renderProjectList();
    } catch (error) {
      if (handleUnauthorized(error)) return;
      if (!silent) showToast("No se pudieron cargar las conversaciones.", { error: true });
    } finally {
      state.workspaceRoomDirectoryLoading.delete(workspaceID);
    }
  }

  function renderWorkspaceOverview(workspace, context, { loading = false, failed = false } = {}) {
    const projectByID = new Map(state.projects.map((project) => [project.id, project]));
    const projects = (workspace.projects || []).map(
      (project) => projectByID.get(project.id) || project,
    );
    const activeProjects = projects.filter((project) => project.status === "active");
    const repositories = projects.filter(
      (project) => project.root_repository || project.root_repository_remote_url,
    );
    const contextItems = workspaceContextItems(context);

    elements.workspaceOverviewName.textContent = valueOrDash(workspace.name);
    elements.workspaceOverviewStatus.textContent =
      projectStatusCopy[workspace.status] || valueOrDash(workspace.status);
    elements.workspaceOverviewDescription.textContent =
      workspace.description ||
      "Frontera compartida de proyectos, contexto y colaboradores.";
    elements.workspaceProjectCount.textContent = formatInteger(projects.length);
    elements.workspaceActiveProjectCount.textContent = formatInteger(activeProjects.length);
    elements.workspaceRepositoryCount.textContent = formatInteger(repositories.length);
    elements.workspaceContextCount.textContent = loading ? "…" : formatInteger(contextItems.length);
    elements.workspaceProjectListCount.textContent = formatInteger(projects.length);

    elements.workspaceProjectList.replaceChildren();
    if (projects.length === 0) {
      const empty = document.createElement("p");
      empty.className = "panel-empty compact-empty";
      empty.textContent = "Este workspace todavía no contiene proyectos.";
      elements.workspaceProjectList.append(empty);
    }
    for (const project of projects) {
      const card = document.createElement("button");
      card.type = "button";
      card.className = "workspace-project-card";
      const heading = document.createElement("span");
      heading.className = "workspace-project-card-heading";
      const name = document.createElement("strong");
      name.textContent = valueOrDash(project.name);
      const status = document.createElement("span");
      status.textContent = projectStatusCopy[project.status] || valueOrDash(project.status);
      heading.append(name, status);
      const repository = document.createElement("code");
      repository.textContent = valueOrDash(
        project.root_repository?.remote_url || project.root_repository_remote_url,
      );
      const action = document.createElement("span");
      action.className = "workspace-project-card-action";
      action.textContent = "Abrir proyecto →";
      card.append(heading, repository, action);
      card.addEventListener("click", () => selectProject(project.id, { focusProject: true }));
      elements.workspaceProjectList.append(card);
    }

    elements.workspaceContextPreview.replaceChildren();
    if (loading) {
      const loadingCopy = document.createElement("p");
      loadingCopy.className = "workspace-context-state";
      loadingCopy.textContent = "Cargando la memoria compartida…";
      elements.workspaceContextPreview.append(loadingCopy);
      return;
    }
    if (failed) {
      const failedCopy = document.createElement("p");
      failedCopy.className = "workspace-context-state is-error";
      failedCopy.textContent = "No se pudo cargar el contexto de este workspace.";
      elements.workspaceContextPreview.append(failedCopy);
      return;
    }
    if (contextItems.length === 0) {
      const emptyCopy = document.createElement("p");
      emptyCopy.className = "workspace-context-state";
      emptyCopy.textContent = "Aún no hay conocimiento durable compartido.";
      elements.workspaceContextPreview.append(emptyCopy);
      return;
    }
    for (const item of contextItems.slice(0, 8)) {
      const card = document.createElement("article");
      card.className = "workspace-context-item";
      const label = document.createElement("span");
      label.textContent = item.label;
      const title = document.createElement("strong");
      title.textContent = valueOrDash(item.title);
      card.append(label, title);
      elements.workspaceContextPreview.append(card);
    }
  }

  function workspaceContextItems(context) {
    if (!context || typeof context !== "object") return [];
    const groups = [
      ["Decisión", context.decisions],
      ["Requisito", context.requirements],
      ["Restricción", context.constraints],
      ["Pregunta", context.open_questions],
      ["Riesgo", context.risks],
      ["Fuente", context.resources],
    ];
    return groups.flatMap(([label, values]) =>
      (Array.isArray(values) ? values : []).map((value) => ({
        label,
        title: value.title || value.body || value.locator,
      })),
    );
  }

  function resetWorkspaceRooms() {
    clearInterval(state.roomPollTimer);
    state.roomPollTimer = null;
    state.roomRequestSequence += 1;
    state.rooms = [];
    state.roomParticipants = [];
    state.selectedRoomID = null;
    state.roomMessages = [];
    state.roomMentions = [];
    state.roomMentionActorIDs = new Set();
    state.replyToMessageID = null;
    if (elements.roomMentionMenu) elements.roomMentionMenu.hidden = true;
  }

  function renderRoomDirectory() {
    elements.workspaceRoomCount.textContent = formatInteger(state.rooms.length);
    elements.workspaceRoomList.replaceChildren();
    const pendingRooms = new Set(state.roomMentions.map((mention) => mention.room_id));

    if (state.rooms.length === 0) {
      const empty = document.createElement("p");
      empty.className = "workspace-context-state";
      empty.textContent = "No hay conversaciones disponibles.";
      elements.workspaceRoomList.append(empty);
      return;
    }

    for (const room of state.rooms) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "workspace-room-button";
      button.classList.toggle("is-selected", room.id === state.selectedRoomID);
      button.setAttribute("aria-current", room.id === state.selectedRoomID ? "page" : "false");
      button.title = room.description || room.name;

      const hash = document.createElement("span");
      hash.textContent = "#";
      hash.setAttribute("aria-hidden", "true");
      const name = document.createElement("strong");
      name.textContent = room.name;
      button.append(hash, name);
      if (pendingRooms.has(room.id)) {
        const unread = document.createElement("small");
        unread.setAttribute("aria-label", "Tiene menciones pendientes");
        button.append(unread);
      }
      button.addEventListener("click", () => selectRoom(room.id));
      elements.workspaceRoomList.append(button);
    }
  }

  async function selectRoom(roomID, { focusMessageID = null } = {}) {
    const room = state.rooms.find((candidate) => candidate.id === roomID);
    if (!room || !state.selectedWorkspaceID || state.selectedProjectID) return;

    state.selectedRoomID = roomID;
    state.workspaceSection = "conversations";
    state.roomMessages = [];
    clearRoomReply();
    renderRoomDirectory();
    renderProjectList();
    renderRoomConversation({ loading: true });
    await loadRoomMessages({ focusMessageID });
  }

  async function loadRoomMessages({ silent = false, focusMessageID = null } = {}) {
    const workspaceID = state.selectedWorkspaceID;
    const roomID = state.selectedRoomID;
    if (!workspaceID || !roomID || state.selectedProjectID) return;
    const sequence = ++state.roomRequestSequence;
    if (!silent) setRoomLiveStatus("loading", "Cargando");

    try {
      const payload = await requestJSON(
        `/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms/${encodeURIComponent(roomID)}/messages?limit=50`,
      );
      if (
        sequence !== state.roomRequestSequence ||
        state.selectedWorkspaceID !== workspaceID ||
        state.selectedRoomID !== roomID ||
        state.selectedProjectID
      ) {
        return;
      }
      const nextMessages = payload?.data?.messages || [];
      if (silent && sameRoomMessages(state.roomMessages, nextMessages)) {
        setRoomLiveStatus("live", "Actualizada");
        return;
      }
      const distanceFromBottom =
        elements.roomMessageList.scrollHeight -
        elements.roomMessageList.scrollTop -
        elements.roomMessageList.clientHeight;
      const previousScrollTop = elements.roomMessageList.scrollTop;
      state.roomMessages = nextMessages;
      renderRoomConversation({
        focusMessageID,
        stickToBottom: !silent || distanceFromBottom < 80,
        scrollTop: previousScrollTop,
      });
      setRoomLiveStatus("live", "Actualizada");
    } catch (error) {
      if (handleUnauthorized(error)) return;
      if (sequence !== state.roomRequestSequence) return;
      setRoomLiveStatus("error", "Sin conexión");
      if (!silent) showToast(requestErrorMessage(error), { error: true });
    }
  }

  function renderRoomConversation({
    loading = false,
    focusMessageID = null,
    stickToBottom = true,
    scrollTop = null,
  } = {}) {
    const room = state.rooms.find((candidate) => candidate.id === state.selectedRoomID);
    elements.roomMessageList.replaceChildren();

    if (!room) {
      elements.selectedRoomName.textContent = "Selecciona una conversación";
      elements.selectedRoomDescription.textContent =
        "Elige un espacio para consultar o enriquecer el contexto compartido.";
      elements.roomMessageEmpty.hidden = false;
      elements.roomMessageInput.disabled = true;
      elements.sendRoomMessage.disabled = true;
      setRoomLiveStatus("stopped", "En espera");
      return;
    }

    elements.selectedRoomName.textContent = `#${room.name}`;
    elements.selectedRoomDescription.textContent =
      room.description || "Conversación durable del workspace.";
    elements.roomMessageInput.disabled = false;
    elements.sendRoomMessage.disabled = false;

    if (loading) {
      elements.roomMessageEmpty.hidden = false;
      elements.roomMessageEmpty.querySelector("h3").textContent = "Cargando conversación…";
      elements.roomMessageEmpty.querySelector("p").textContent =
        "Pact recupera únicamente los últimos mensajes de esta conversación.";
      return;
    }

    const hasMessages = state.roomMessages.length > 0;
    elements.roomMessageEmpty.hidden = hasMessages;
    elements.roomMessageEmpty.querySelector("h3").textContent = "Esta conversación todavía está en silencio";
    elements.roomMessageEmpty.querySelector("p").textContent =
      "Comparte una decisión, una reunión o menciona a alguien para pedirle contexto.";

    for (const message of state.roomMessages) {
      const article = document.createElement("article");
      article.className = "room-message";
      article.classList.toggle("is-agent", message.author_kind === "agent");
      article.dataset.messageId = message.id;

      const avatar = document.createElement("span");
      avatar.className = "room-message-avatar";
      avatar.textContent = initials(message.author_display_name);
      avatar.setAttribute("aria-hidden", "true");

      const content = document.createElement("div");
      const heading = document.createElement("div");
      heading.className = "room-message-heading";
      const author = document.createElement("strong");
      author.textContent = valueOrDash(message.author_display_name);
      const kind = document.createElement("span");
      kind.className = "room-author-kind";
      kind.textContent = message.author_kind === "agent" ? "IA" : "Persona";
      const time = document.createElement("time");
      time.dateTime = message.created_at || "";
      time.textContent = formatRelativeDate(message.created_at);
      time.title = formatDateTime(message.created_at);
      heading.append(author, kind, time);

      const body = document.createElement("p");
      body.className = "room-message-body";
      body.textContent = message.body;
      content.append(heading, body);

      const meta = document.createElement("div");
      meta.className = "room-message-meta";
      if (message.reply_to_message_id) {
        const replyLabel = document.createElement("span");
        replyLabel.className = "room-message-mention";
        replyLabel.textContent = "Respuesta";
        meta.append(replyLabel);
      }
      for (const mention of message.mentions || []) {
        const chip = document.createElement("span");
        chip.className = "room-message-mention";
        chip.textContent = `@${participantHandle(mention.actor_id, mention.display_name)}`;
        meta.append(chip);
      }
      const reply = document.createElement("button");
      reply.type = "button";
      reply.className = "room-message-reply";
      reply.textContent = "Responder";
      reply.addEventListener("click", () => setRoomReply(message));
      meta.append(reply);
      content.append(meta);
      article.append(avatar, content);
      elements.roomMessageList.append(article);
    }

    if (focusMessageID) {
      elements.roomMessageList
        .querySelector(`[data-message-id="${CSS.escape(focusMessageID)}"]`)
        ?.scrollIntoView({ block: "center" });
    } else if (stickToBottom) {
      elements.roomMessageList.scrollTop = elements.roomMessageList.scrollHeight;
    } else if (scrollTop !== null) {
      elements.roomMessageList.scrollTop = scrollTop;
    }
  }

  function sameRoomMessages(left, right) {
    if (left.length !== right.length) return false;
    return left.every((message, index) => {
      const candidate = right[index];
      if (!candidate || message.id !== candidate.id || message.body !== candidate.body) return false;
      const mentionState = (message.mentions || [])
        .map((mention) => `${mention.mention_id}:${mention.status}`)
        .join("|");
      const candidateMentionState = (candidate.mentions || [])
        .map((mention) => `${mention.mention_id}:${mention.status}`)
        .join("|");
      return mentionState === candidateMentionState;
    });
  }

  function startRoomPolling() {
    clearInterval(state.roomPollTimer);
    state.roomPollTimer = window.setInterval(() => {
      if (document.hidden || !state.selectedRoomID || state.selectedProjectID) return;
      loadRoomMessages({ silent: true });
      loadRoomMentions({ silent: true });
    }, ROOM_POLL_MS);
  }

  function setRoomLiveStatus(kind, label) {
    elements.roomLiveStatus.classList.toggle("is-live", kind === "live");
    elements.roomLiveStatus.classList.toggle("is-error", kind === "error");
    const textNode = [...elements.roomLiveStatus.childNodes]
      .reverse()
      .find((node) => node.nodeType === Node.TEXT_NODE);
    if (textNode) textNode.textContent = ` ${label}`;
  }

  function openCreateRoomDialog() {
    if (!state.selectedWorkspaceID || state.selectedProjectID) return;
    elements.createRoomForm.reset();
    elements.createRoomDialog.showModal();
    window.setTimeout(() => elements.createRoomName.focus(), 0);
  }

  function closeCreateRoomDialog() {
    elements.createRoomDialog.close();
  }

  async function createRoom(event) {
    event.preventDefault();
    const workspaceID = state.selectedWorkspaceID;
    const name = elements.createRoomName.value.trim();
    const description = elements.createRoomDescription.value.trim();
    if (!workspaceID || !name) {
      elements.createRoomName.focus();
      return;
    }
    setBusy(elements.createRoomSubmit, true);
    try {
      const payload = await requestJSON(
        `/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms`,
        {
          method: "POST",
          headers: { "Idempotency-Key": createIdempotencyKey("room") },
          body: { name, description },
        },
      );
      if (state.selectedWorkspaceID !== workspaceID || state.selectedProjectID) return;
      const room = payload?.data;
      if (room) state.rooms.push(room);
      state.workspaceRoomDirectories.set(workspaceID, [...state.rooms]);
      closeCreateRoomDialog();
      renderRoomDirectory();
      renderProjectList();
      showToast(`Conversación #${room?.name || name} creada.`);
      if (room) await selectRoom(room.id);
    } catch (error) {
      handleRequestFailure(error, "No se pudo crear la conversación.");
    } finally {
      setBusy(elements.createRoomSubmit, false);
    }
  }

  async function postRoomMessage(event) {
    event.preventDefault();
    const workspaceID = state.selectedWorkspaceID;
    const roomID = state.selectedRoomID;
    const body = elements.roomMessageInput.value.trim();
    if (!workspaceID || !roomID || !body) {
      elements.roomMessageInput.focus();
      return;
    }

    const mentionActorIDs = [...state.roomMentionActorIDs].filter((actorID) => {
      const participant = state.roomParticipants.find((item) => item.actor_id === actorID);
      return participant && body.includes(`@${participant.handle}`);
    });
    setBusy(elements.sendRoomMessage, true);
    try {
      const payload = await requestJSON(
        `/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms/${encodeURIComponent(roomID)}/messages`,
        {
          method: "POST",
          headers: { "Idempotency-Key": createIdempotencyKey("message") },
          body: {
            body,
            reply_to_message_id: state.replyToMessageID || undefined,
            mention_actor_ids: mentionActorIDs,
          },
        },
      );
      if (
        state.selectedWorkspaceID !== workspaceID ||
        state.selectedRoomID !== roomID ||
        state.selectedProjectID
      ) {
        return;
      }
      const message = payload?.data;
      if (message && !state.roomMessages.some((item) => item.id === message.id)) {
        state.roomMessages.push(message);
      }
      elements.roomMessageInput.value = "";
      state.roomMentionActorIDs.clear();
      clearRoomReply();
      renderRoomConversation();
      elements.roomMessageInput.focus();
    } catch (error) {
      handleRequestFailure(error, "No se pudo enviar el mensaje.");
    } finally {
      setBusy(elements.sendRoomMessage, false);
    }
  }

  function renderRoomMentionMenu() {
    const input = elements.roomMessageInput;
    const beforeCursor = input.value.slice(0, input.selectionStart || input.value.length);
    const match = beforeCursor.match(/(^|\s)@([a-z0-9-]*)$/i);
    if (!match) {
      elements.roomMentionMenu.hidden = true;
      return;
    }
    const query = match[2].toLocaleLowerCase("es");
    const participants = state.roomParticipants
      .filter((participant) =>
        [participant.handle, participant.display_name].some((value) =>
          String(value || "").toLocaleLowerCase("es").includes(query),
        ),
      )
      .slice(0, 10);
    elements.roomMentionMenu.replaceChildren();
    if (participants.length === 0) {
      elements.roomMentionMenu.hidden = true;
      return;
    }
    for (const participant of participants) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "room-mention-option";
      button.setAttribute("role", "option");
      const avatar = document.createElement("span");
      avatar.textContent = initials(participant.display_name);
      const copy = document.createElement("span");
      const name = document.createElement("strong");
      name.textContent = participant.display_name;
      const handle = document.createElement("small");
      handle.textContent = `@${participant.handle}`;
      copy.append(name, handle);
      const kind = document.createElement("em");
      kind.textContent = participant.kind === "agent"
        ? participant.online ? "IA · activa" : "IA"
        : "Persona";
      button.append(avatar, copy, kind);
      button.addEventListener("click", () => selectRoomMention(participant, match));
      elements.roomMentionMenu.append(button);
    }
    elements.roomMentionMenu.hidden = false;
  }

  function selectRoomMention(participant, match) {
    const input = elements.roomMessageInput;
    const cursor = input.selectionStart || input.value.length;
    const beforeCursor = input.value.slice(0, cursor);
    const mentionStart = beforeCursor.length - match[2].length - 1;
    const replacement = `@${participant.handle} `;
    input.value =
      input.value.slice(0, mentionStart) + replacement + input.value.slice(cursor);
    const nextCursor = mentionStart + replacement.length;
    input.setSelectionRange(nextCursor, nextCursor);
    state.roomMentionActorIDs.add(participant.actor_id);
    elements.roomMentionMenu.hidden = true;
    input.focus();
  }

  function handleRoomComposerKeydown(event) {
    if (event.key === "Escape" && !elements.roomMentionMenu.hidden) {
      elements.roomMentionMenu.hidden = true;
      event.preventDefault();
      return;
    }
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      elements.roomMessageForm.requestSubmit();
    }
  }

  function setRoomReply(message) {
    state.replyToMessageID = message.id;
    elements.roomReplyCopy.textContent = `Respondiendo a ${message.author_display_name}: ${truncateText(message.body, 90)}`;
    elements.roomReplyPreview.hidden = false;
    elements.roomMessageInput.focus();
  }

  function clearRoomReply() {
    state.replyToMessageID = null;
    elements.roomReplyPreview.hidden = true;
    elements.roomReplyCopy.textContent = "";
  }

  async function loadRoomMentions({ silent = false } = {}) {
    const workspaceID = state.selectedWorkspaceID;
    if (!workspaceID || state.selectedProjectID) return;
    try {
      const payload = await requestJSON(
        `/v1/me/room-mentions?workspace_id=${encodeURIComponent(workspaceID)}&status=pending&limit=100`,
      );
      if (state.selectedWorkspaceID !== workspaceID || state.selectedProjectID) return;
      state.roomMentions = (payload?.data?.mentions || []).filter(
        (mention) => mention.workspace_id === workspaceID,
      );
      renderRoomMentionInbox();
      renderRoomDirectory();
      await loadGlobalRoomMentions({ silent: true });
    } catch (error) {
      if (!silent) handleRequestFailure(error, "No se pudieron cargar las menciones.");
    }
  }

  async function loadGlobalRoomMentions({ silent = false } = {}) {
    if (!state.token) return;
    try {
      const payload = await requestJSON("/v1/me/room-mentions?status=pending&limit=100");
      const nextMentions = payload?.data?.mentions || [];
      const directoryChanged = roomMentionDirectoryKey(state.globalRoomMentions) !==
        roomMentionDirectoryKey(nextMentions);
      state.globalRoomMentions = nextMentions;
      renderGlobalRoomMentionCount();
      if (directoryChanged) renderProjectList();
      if (state.mentionDialogScope === "global" && elements.roomMentionsDialog.open) {
        renderRoomMentionInbox({ mentions: state.globalRoomMentions, global: true });
      }
    } catch (error) {
      if (handleUnauthorized(error)) return;
      if (!silent) showToast("No se pudieron cargar las menciones.", { error: true });
    }
  }

  function roomMentionDirectoryKey(mentions) {
    return mentions
      .map((mention) => `${mention.id}:${mention.workspace_id}:${mention.room_id}:${mention.status}`)
      .sort()
      .join("|");
  }

  function renderGlobalRoomMentionCount() {
    const count = state.globalRoomMentions.length;
    elements.globalRoomMentionCount.textContent = count > 99 ? "99+" : String(count);
    elements.globalRoomMentions.classList.toggle("has-mentions", count > 0);
    elements.globalRoomMentions.setAttribute(
      "aria-label",
      count === 1 ? "Abrir 1 mención pendiente" : `Abrir ${count} menciones pendientes`,
    );
  }

  function renderRoomMentionInbox({ mentions = state.roomMentions, global = false } = {}) {
    elements.roomMentionCount.textContent = formatInteger(state.roomMentions.length);
    elements.roomMentionList.replaceChildren();
    if (mentions.length === 0) {
      const empty = document.createElement("p");
      empty.className = "workspace-context-state";
      empty.textContent = global
        ? "No tienes menciones pendientes."
        : "No tienes menciones pendientes en este workspace.";
      elements.roomMentionList.append(empty);
      return;
    }
    for (const mention of mentions) {
      const item = document.createElement("article");
      item.className = "room-inbox-item";
      const header = document.createElement("header");
      const author = document.createElement("strong");
      author.textContent = `${mention.message.author_display_name} · #${mention.room_name}`;
      const time = document.createElement("span");
      time.textContent = formatRelativeDate(mention.created_at);
      header.append(author, time);
      const body = document.createElement("p");
      body.textContent = mention.message.body;
      const actions = document.createElement("div");
      actions.className = "room-inbox-actions";
      const open = document.createElement("button");
      open.type = "button";
      open.className = "room-mention-action";
      open.textContent = "Abrir conversación";
      open.addEventListener("click", () => openRoomMention(mention));
      const dismiss = document.createElement("button");
      dismiss.type = "button";
      dismiss.className = "room-mention-action";
      dismiss.textContent = "Descartar";
      dismiss.addEventListener("click", () => updateRoomMention(mention.id, "dismissed"));
      actions.append(open, dismiss);
      item.append(header, body, actions);
      elements.roomMentionList.append(item);
    }
  }

  function openRoomMentionsDialog() {
    state.mentionDialogScope = "workspace";
    const workspace = state.workspaces.find((item) => item.id === state.selectedWorkspaceID);
    elements.roomMentionsKicker.textContent = "WORKSPACE";
    elements.roomMentionsTitle.textContent = workspace
      ? `Menciones en ${workspace.name}`
      : "Menciones pendientes";
    renderRoomMentionInbox();
    elements.roomMentionsDialog.showModal();
  }

  function openGlobalRoomMentionsDialog() {
    state.mentionDialogScope = "global";
    elements.roomMentionsKicker.textContent = "PARA TI";
    elements.roomMentionsTitle.textContent = "Menciones pendientes";
    renderRoomMentionInbox({ mentions: state.globalRoomMentions, global: true });
    elements.roomMentionsDialog.showModal();
  }

  async function openRoomMention(mention) {
    elements.roomMentionsDialog.close();
    if (state.selectedWorkspaceID !== mention.workspace_id || state.selectedProjectID) {
      await openWorkspaceConversation(mention.workspace_id, mention.room_id, {
        focusMessageID: mention.message.id,
      });
    } else {
      await selectRoom(mention.room_id, { focusMessageID: mention.message.id });
      focusWorkspaceSection("conversations", { focusComposer: true });
    }
    await updateRoomMention(mention.id, "read", { announce: false });
    const message = state.roomMessages.find((item) => item.id === mention.message.id);
    if (message) setRoomReply(message);
  }

  async function updateRoomMention(mentionID, status, { announce = true } = {}) {
    try {
      await requestJSON(`/v1/me/room-mentions/${encodeURIComponent(mentionID)}/status`, {
        method: "POST",
        body: { status },
      });
      state.roomMentions = state.roomMentions.filter((mention) => mention.id !== mentionID);
      state.globalRoomMentions = state.globalRoomMentions.filter(
        (mention) => mention.id !== mentionID,
      );
      renderGlobalRoomMentionCount();
      renderRoomMentionInbox(
        state.mentionDialogScope === "global"
          ? { mentions: state.globalRoomMentions, global: true }
          : undefined,
      );
      renderRoomDirectory();
      renderProjectList();
      if (announce) showToast(status === "dismissed" ? "Mención descartada." : "Mención leída.");
    } catch (error) {
      handleRequestFailure(error, "No se pudo actualizar la mención.");
    }
  }

  function participantHandle(actorID, displayName) {
    return state.roomParticipants.find((participant) => participant.actor_id === actorID)?.handle ||
      String(displayName || "actor").toLocaleLowerCase("es").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  }

  function initials(value) {
    return String(value || "?")
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part[0] || "")
      .join("")
      .toLocaleUpperCase("es") || "?";
  }

  function truncateText(value, maximum) {
    const text = String(value || "").replace(/\s+/g, " ").trim();
    return text.length > maximum ? `${text.slice(0, maximum - 1)}…` : text;
  }

  function createIdempotencyKey(prefix) {
    if (globalThis.crypto?.randomUUID) return `${prefix}-${globalThis.crypto.randomUUID()}`;
    return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }

  function clearSelection() {
    stopLiveUpdates();
    state.selectedWorkspaceID = null;
    state.selectedProjectID = null;
    state.workspaceSection = "overview";
    state.workspaceContext = null;
    resetWorkspaceRooms();
    state.overview = null;
    state.projectAccess = null;
    state.knowledge = null;
    state.projectRepositories = [];
    state.repositorySyncStates = [];
    state.availableRepositories = [];
    state.events = [];
    elements.workspaceOverview.hidden = true;
    elements.workspaceContent.hidden = true;
    elements.workspaceEmpty.hidden = false;
    renderProjectList();
  }

  function renderProjectHeader(project) {
    const workspace = state.workspaces.find((item) =>
      (item.projects || []).some((candidate) => candidate.id === project.id),
    );
    elements.workspaceName.textContent = workspace ? valueOrDash(workspace.name) : "Sin workspace";
    elements.projectName.textContent = valueOrDash(project.name);
    elements.projectSlug.textContent = valueOrDash(project.slug);
    elements.projectID.textContent = valueOrDash(project.id);
    elements.projectID.title = project.id || "";
    elements.settingsWorkspace.textContent = workspace
      ? valueOrDash(workspace.name)
      : "Sin workspace";
    elements.settingsRole.textContent =
      organizationRoleCopy[state.principal?.organization_role] ||
      valueOrDash(state.principal?.organization_role);
    elements.settingsProjectID.textContent = valueOrDash(project.id);
    elements.settingsProjectID.title = project.id || "";

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

  function setDashboardView(view, { focus = false } = {}) {
    if (!Object.hasOwn(dashboardViews, view)) return;
    state.activeView = view;
    const copy = dashboardViews[view];
    elements.dashboardViewKicker.textContent = copy.kicker;
    elements.dashboardViewTitle.textContent = copy.title;
    elements.dashboardViewDescription.textContent = copy.description;
    elements.openProjectSettings.classList.toggle("is-active", view === "settings");
    elements.openProjectSettings.setAttribute("aria-pressed", String(view === "settings"));

    const dashboardTabs = [...elements.projectTabs.querySelectorAll("[data-dashboard-view]")];
    for (const tab of dashboardTabs) {
      const selected = tab.dataset.dashboardView === view;
      tab.setAttribute("aria-selected", String(selected));
      tab.tabIndex = selected ? 0 : -1;
      if (selected && focus) tab.focus();
    }
    if (view === "settings" && dashboardTabs[0]) dashboardTabs[0].tabIndex = 0;
    for (const panel of elements.dashboardPanels) {
      const sections = String(panel.dataset.dashboardSections || "")
        .split(/\s+/)
        .filter(Boolean);
      panel.hidden = !sections.includes(view);
    }
  }

  function handleDashboardTabKey(event) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const tabs = [...elements.projectTabs.querySelectorAll("[data-dashboard-view]")];
    const current = tabs.indexOf(event.currentTarget);
    if (current < 0) return;
    event.preventDefault();
    let next = current;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = tabs.length - 1;
    if (event.key === "ArrowLeft") next = (current - 1 + tabs.length) % tabs.length;
    if (event.key === "ArrowRight") next = (current + 1) % tabs.length;
    setDashboardView(tabs[next].dataset.dashboardView, { focus: true });
  }

  function resetOverview() {
    renderAttention(null);
    renderActivity(null);
    renderRepositorySync(null);
    renderProjectRepositories();
    renderCounts(null);
    renderActiveWork([]);
    renderWorkItems([]);
    renderKnowledge(null);
    renderProjectAccess(null);
    renderHandoffs([]);
    renderEvents([]);
    elements.tabLiveCount.textContent = "—";
    elements.tabWorkCount.textContent = "—";
    elements.tabKnowledgeCount.textContent = "—";
    elements.tabAccessCount.textContent = "—";
    elements.tabRepositoryCount.textContent = "—";
    elements.tabActivityCount.textContent = "—";
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
      const workspace = workspaceForProject(projectID);
      const [payload, knowledgePayload, accessPayload] = await Promise.all([
        requestJSON(`/v1/projects/${encodeURIComponent(projectID)}/overview`),
        workspace
          ? requestJSON(`/v1/workspaces/${encodeURIComponent(workspace.id)}/context`)
          : Promise.resolve(null),
        requestJSON(`/v1/projects/${encodeURIComponent(projectID)}/access`),
      ]);
      if (
        requestSequence !== state.overviewRequestSequence ||
        projectID !== state.selectedProjectID ||
        requestToken !== state.token
      ) {
        return;
      }

      const overview = payload && payload.data ? payload.data : payload;
      state.overview = overview || {};
      state.knowledge = knowledgePayload
        ? (knowledgePayload.data || knowledgePayload)
        : null;
      state.projectAccess = accessPayload
        ? (accessPayload.data || accessPayload)
        : null;
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
    renderAttention(overview);
    renderActivity(overview.code_activity);
    renderRepositorySync(overview.repository_sync);
    renderCounts(overview.counts);
    renderActiveWork(
      Array.isArray(overview.active_work) ? overview.active_work : [],
    );
    renderWorkItems(
      Array.isArray(overview.work_items) ? overview.work_items : [],
    );
    renderKnowledge(state.knowledge);
    renderProjectAccess(state.projectAccess);
    renderHandoffs(
      Array.isArray(overview.handoffs) ? overview.handoffs : [],
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

  function renderRepositorySync(sync) {
    const value = sync && typeof sync === "object" ? sync : {};
    const status = value.status || "never";
    const copy = {
      never: ["Sin verificar", "Pact aún no ha verificado la rama canónica directamente con GitHub."],
      synced: ["Verificado", "La revisión canónica fue leída directamente desde GitHub."],
      failed: ["Con error", "GitHub no pudo confirmar el estado canónico. Revisa la credencial o la disponibilidad del repositorio."],
      unsupported: ["No compatible", "Este remoto todavía no utiliza un proveedor compatible con la sincronización canónica."],
      unavailable: ["No disponible", "El proyecto no tiene un repositorio raíz activo que Pact pueda verificar."],
    };
    const [label, description] = copy[status] || [status, "Estado de sincronización desconocido."];
    elements.repositorySyncPanel.className = `repository-sync-panel state-${status}`;
    elements.repositorySyncStatus.textContent = label;
    elements.repositorySyncDescription.textContent = description;
    elements.repositorySyncName.textContent = valueOrDash(value.repository_full_name);
    elements.repositorySyncBranch.textContent = valueOrDash(value.default_branch);
    elements.repositorySyncRevision.textContent = value.canonical_revision
      ? shortenIdentifier(value.canonical_revision, 16)
      : "—";
    elements.repositorySyncRevision.title = value.canonical_revision || "";
    const timestamp = value.last_success_at || value.last_attempt_at;
    elements.repositorySyncTime.textContent = timestamp
      ? formatRelativeDate(timestamp)
      : "Nunca";
    elements.repositorySyncTime.title = timestamp || "";
    elements.syncRepository.disabled = ["unsupported", "unavailable"].includes(status);
  }

  function renderGitHubStatus() {
    const status = state.githubStatus && typeof state.githubStatus === "object"
      ? state.githubStatus
      : {};
    const installations = Array.isArray(status.installations)
      ? status.installations.filter((installation) => installation.status !== "deleted")
      : [];
    const active = installations.filter((installation) => installation.status === "active");
    const connected = active.length > 0;
    const canManage = ["owner", "admin"].includes(state.principal?.organization_role);
    elements.githubConnectionCard.classList.toggle("is-connected", connected);
    if (!status.configured) {
      elements.githubConnectionStatus.textContent = "No configurado";
      elements.githubConnectionDescription.textContent =
        "El servidor necesita las credenciales de una GitHub App antes de iniciar la conexión.";
      elements.connectGitHub.textContent = "Configurar GitHub App";
      elements.connectGitHub.disabled = true;
      return;
    }
    elements.connectGitHub.disabled = !canManage;
    if (connected) {
      const accounts = active.map((installation) => installation.account_login).join(", ");
      elements.githubConnectionStatus.textContent = "Conectado";
      elements.githubConnectionDescription.textContent =
        `${accounts || "GitHub"} · ${formatInteger(status.repository_count || 0)} repositorios autorizados`;
      elements.connectGitHub.textContent = "Administrar acceso";
    } else {
      elements.githubConnectionStatus.textContent = installations.length > 0 ? "Suspendido" : "Sin conectar";
      elements.githubConnectionDescription.textContent = installations.length > 0
        ? "La instalación no está activa. Reautoriza Pact en GitHub."
        : "Autoriza una cuenta y selecciona los repositorios que Pact podrá leer.";
      elements.connectGitHub.textContent = "Conectar GitHub";
    }
    if (!canManage) {
      elements.connectGitHub.textContent = "Requiere administrador";
    }
  }

  async function connectGitHub() {
    if (!state.token) return;
    setBusy(elements.connectGitHub, true);
    try {
      const payload = await requestJSON("/v1/integrations/github/connect", {
        method: "POST",
        body: {},
      });
      const connection = payload?.data || payload;
      if (!connection?.install_url) {
        throw new APIError("Pact no devolvió una URL de instalación válida.");
      }
      window.location.assign(connection.install_url);
    } catch (error) {
      handleRequestFailure(error, "No se pudo iniciar la conexión con GitHub.");
      setBusy(elements.connectGitHub, false);
    }
  }

  function announceGitHubCallback() {
    const currentURL = new URL(window.location.href);
    const result = currentURL.searchParams.get("github");
    if (!result) return;
    if (result === "connected") {
      showToast("GitHub quedó conectado y los repositorios autorizados ya están disponibles.");
    } else {
      showToast("No se pudo completar la conexión con GitHub.", { error: true });
    }
    currentURL.searchParams.delete("github");
    currentURL.searchParams.delete("reason");
    window.history.replaceState({}, "", currentURL.pathname + currentURL.search + currentURL.hash);
  }

  async function loadProjectRepositories({ silent = false } = {}) {
    const projectID = state.selectedProjectID;
    if (!projectID || !state.token) return;
    try {
      const [projectPayload, availablePayload] = await Promise.all([
        requestJSON(`/v1/projects/${encodeURIComponent(projectID)}/repositories`),
        requestJSON(`/v1/integrations/github/repositories?project_id=${encodeURIComponent(projectID)}`),
      ]);
      if (projectID !== state.selectedProjectID) return;
      const projectData = projectPayload?.data || projectPayload || {};
      state.projectRepositories = Array.isArray(projectData.repositories)
        ? projectData.repositories
        : [];
      state.repositorySyncStates = Array.isArray(projectData.sync_states)
        ? projectData.sync_states
        : [];
      const availableData = availablePayload?.data || availablePayload;
      state.availableRepositories = Array.isArray(availableData) ? availableData : [];
      renderProjectRepositories();
    } catch (error) {
      if (!silent) handleRequestFailure(error, "No se pudieron cargar los repositorios del proyecto.");
      if (!handleUnauthorized(error)) {
        state.projectRepositories = [];
        state.repositorySyncStates = [];
        state.availableRepositories = [];
        renderProjectRepositories();
      }
    }
  }

  function renderProjectRepositories() {
    const repositories = state.projectRepositories;
    const states = new Map(state.repositorySyncStates.map((item) => [item.repository_id, item]));
    elements.projectRepositoryCount.textContent = formatInteger(repositories.length);
    elements.tabRepositoryCount.textContent = formatInteger(repositories.length);
    elements.projectRepositoryList.replaceChildren();

    const unattached = state.availableRepositories.filter((repository) => !repository.attached_repository_id);
    elements.availableRepository.replaceChildren();
    const placeholder = document.createElement("option");
    placeholder.value = "";
    placeholder.textContent = unattached.length > 0
      ? "Selecciona un repositorio"
      : "No hay repositorios disponibles";
    elements.availableRepository.append(placeholder);
    for (const repository of unattached) {
      const option = document.createElement("option");
      option.value = String(repository.github_repository_id);
      option.textContent = `${repository.full_name}${repository.private ? " · privado" : ""}`;
      elements.availableRepository.append(option);
    }
    const githubConnected = (state.githubStatus?.installations || []).some(
      (installation) => installation.status === "active",
    );
    elements.attachRepository.disabled = !githubConnected || unattached.length === 0;
    elements.repositoryAttachHint.textContent = !githubConnected
      ? "Conecta GitHub para elegir entre los repositorios autorizados para esta organización."
      : unattached.length === 0
        ? "Todos los repositorios autorizados ya están vinculados o GitHub no tiene otros repositorios seleccionados."
        : "El propósito es flexible: frontend, backend, mobile, infra, docs u otro identificador propio.";

    if (repositories.length === 0) {
      const empty = document.createElement("p");
      empty.className = "panel-empty";
      empty.textContent = "Este proyecto todavía no tiene repositorios operativos.";
      elements.projectRepositoryList.append(empty);
      return;
    }

    for (const repository of repositories) {
      const sync = states.get(repository.id) || {};
      const card = document.createElement("article");
      card.className = `project-repository-card${repository.primary ? " is-primary" : ""}`;

      const content = document.createElement("div");
      const title = document.createElement("h3");
      title.textContent = repository.github_full_name || repository.name || repository.slug;
      title.title = repository.github_full_name || repository.remote_url || "";
      const meta = document.createElement("p");
      meta.className = "project-repository-meta";
      meta.textContent = `${repository.default_branch || "rama desconocida"} · ${sync.status || "sin verificar"}`;
      const revision = document.createElement("p");
      revision.className = "project-repository-revision";
      const code = document.createElement("code");
      code.textContent = sync.canonical_revision
        ? shortenIdentifier(sync.canonical_revision, 14)
        : "Sin revisión";
      code.title = sync.canonical_revision || "";
      revision.append(code);
      const badges = document.createElement("div");
      badges.className = "repository-card-badges";
      for (const label of [
        repository.purpose,
        repository.primary ? "principal" : "adicional",
        repository.required ? "necesario" : "opcional",
        repository.visibility,
      ].filter(Boolean)) {
        const badge = document.createElement("span");
        badge.textContent = label;
        badges.append(badge);
      }
      content.append(title, meta, revision, badges);

      const syncButton = document.createElement("button");
      syncButton.type = "button";
      syncButton.className = "secondary-button";
      syncButton.textContent = "Verificar";
      syncButton.disabled = ["unsupported", "unavailable"].includes(sync.status);
      syncButton.addEventListener("click", () => syncProjectRepository(repository.id, syncButton));
      card.append(content, syncButton);
      elements.projectRepositoryList.append(card);
    }
  }

  async function attachRepository(event) {
    event.preventDefault();
    const projectID = state.selectedProjectID;
    const githubRepositoryID = Number(elements.availableRepository.value);
    if (!projectID || !Number.isSafeInteger(githubRepositoryID) || githubRepositoryID <= 0) return;
    setBusy(elements.attachRepository, true);
    try {
      await requestJSON(`/v1/projects/${encodeURIComponent(projectID)}/repositories`, {
        method: "POST",
        body: {
          github_repository_id: githubRepositoryID,
          purpose: elements.repositoryPurpose.value.trim(),
          required: elements.repositoryRequired.checked,
          primary: elements.repositoryPrimary.checked,
        },
      });
      showToast("El repositorio quedó vinculado al proyecto.");
      elements.repositoryPrimary.checked = false;
      await Promise.all([loadProjectRepositories(), loadOverview({ silent: true }), refreshProjects()]);
    } catch (error) {
      handleRequestFailure(error, "No se pudo vincular el repositorio.");
    } finally {
      setBusy(elements.attachRepository, false);
      renderProjectRepositories();
    }
  }

  async function syncProjectRepository(repositoryID, button) {
    const projectID = state.selectedProjectID;
    if (!projectID) return;
    setBusy(button, true);
    try {
      const key = globalThis.crypto?.randomUUID
        ? `pact-admin-repository-sync-${globalThis.crypto.randomUUID()}`
        : `pact-admin-repository-sync-${Date.now()}`;
      const payload = await requestJSON(
        `/v1/projects/${encodeURIComponent(projectID)}/repositories/${encodeURIComponent(repositoryID)}/sync`,
        { method: "POST", headers: { "Idempotency-Key": key }, body: {} },
      );
      const result = payload?.data || payload;
      showToast(result?.changed ? "GitHub confirmó una nueva revisión." : "El repositorio ya estaba actualizado.");
      await Promise.all([loadProjectRepositories(), loadOverview({ silent: true }), refreshProjects()]);
    } catch (error) {
      handleRequestFailure(error, "No se pudo verificar el repositorio con GitHub.");
      await loadProjectRepositories({ silent: true });
    } finally {
      setBusy(button, false);
    }
  }

  async function syncCanonicalRepository() {
    const projectID = state.selectedProjectID;
    if (!projectID || !state.token) return;
    setBusy(elements.syncRepository, true);
    try {
      const idempotencyKey = globalThis.crypto?.randomUUID
        ? `pact-admin-repository-sync-${globalThis.crypto.randomUUID()}`
        : `pact-admin-repository-sync-${Date.now()}`;
      const payload = await requestJSON(
        `/v1/projects/${encodeURIComponent(projectID)}/repository-sync`,
        {
          method: "POST",
          headers: { "Idempotency-Key": idempotencyKey },
          body: {},
        },
      );
      const result = payload && payload.data ? payload.data : payload;
      renderRepositorySync(result?.state || null);
      showToast(result?.changed
        ? "GitHub confirmó una nueva revisión canónica."
        : "El repositorio canónico ya estaba actualizado.");
      await Promise.all([refreshProjects(), loadOverview({ silent: true })]);
    } catch (error) {
      handleRequestFailure(error, "No se pudo verificar el repositorio con GitHub.");
      await loadOverview({ silent: true });
    } finally {
      setBusy(elements.syncRepository, false);
      if (state.overview?.repository_sync) {
        renderRepositorySync(state.overview.repository_sync);
      }
    }
  }

  function workspaceForProject(projectID) {
    return state.workspaces.find((workspace) =>
      (workspace.projects || []).some((project) => project.id === projectID),
    ) || null;
  }

  function renderKnowledge(context) {
    elements.knowledgeGrid.replaceChildren();
    const value = context && typeof context === "object" ? context : {};
    const groups = [
      {
        title: "Acuerdos vigentes",
        items: [
          ...tagKnowledge(value.decisions, "Decisión"),
          ...tagKnowledge(value.requirements, "Requisito"),
          ...tagKnowledge(value.constraints, "Restricción"),
        ],
      },
      {
        title: "Atención",
        items: [
          ...tagKnowledge(value.open_questions, "Pregunta"),
          ...tagKnowledge(value.risks, "Riesgo"),
        ],
        warnings: Array.isArray(value.warnings) ? value.warnings : [],
      },
      {
        title: "Fuentes registradas",
        resources: Array.isArray(value.resources) ? value.resources : [],
      },
    ];
    const total = groups.reduce(
      (count, group) => count + (group.items?.length || 0) + (group.resources?.length || 0),
      0,
    );
    elements.knowledgeCount.textContent = context ? formatInteger(total) : "—";
    elements.tabKnowledgeCount.textContent = context ? formatInteger(total) : "—";
    elements.knowledgeEmpty.hidden = !context || total !== 0;
    elements.knowledgeGrid.hidden = !context || total === 0;
    if (!context || total === 0) return;

    for (const group of groups) {
      const lane = document.createElement("section");
      lane.className = "knowledge-lane";
      const heading = document.createElement("h3");
      heading.textContent = group.title;
      lane.append(heading);
      const items = group.items || [];
      for (const item of items.slice(0, 6)) {
        const card = document.createElement("article");
        card.className = `knowledge-item status-${item.status || "active"}`;
        const type = document.createElement("span");
        type.textContent = `${item.label} · ${item.status || "propuesto"}`;
        const title = document.createElement("strong");
        title.textContent = valueOrDash(item.title);
        const body = document.createElement("p");
        body.textContent = valueOrDash(item.body);
        card.append(type, title, body);
        lane.append(card);
      }
      for (const resource of (group.resources || []).slice(0, 6)) {
        const card = document.createElement("article");
        card.className = "knowledge-item resource-item";
        const type = document.createElement("span");
        type.textContent = `${resource.kind || "fuente"} · ${resource.classification || "internal"}`;
        const title = document.createElement("strong");
        title.textContent = valueOrDash(resource.title);
        const locator = document.createElement("code");
        locator.textContent = valueOrDash(resource.locator);
        locator.title = resource.locator || "";
        card.append(type, title, locator);
        lane.append(card);
      }
      for (const warning of (group.warnings || []).slice(0, 3)) {
        const note = document.createElement("p");
        note.className = "knowledge-warning";
        note.textContent = warning;
        lane.append(note);
      }
      if (items.length === 0 && (group.resources || []).length === 0 && (group.warnings || []).length === 0) {
        const empty = document.createElement("p");
        empty.className = "knowledge-lane-empty";
        empty.textContent = "Sin registros";
        lane.append(empty);
      }
      elements.knowledgeGrid.append(lane);
    }
  }

  function tagKnowledge(items, label) {
    return (Array.isArray(items) ? items : []).map((item) => ({ ...item, label }));
  }

  function renderHandoffs(handoffs) {
    elements.handoffList.replaceChildren();
    elements.handoffCount.textContent = formatInteger(handoffs.length);
    elements.handoffEmpty.hidden = handoffs.length !== 0;
    elements.handoffList.hidden = handoffs.length === 0;

    for (const handoff of handoffs) {
      const card = document.createElement("article");
      card.className = `handoff-item status-${handoff.status || "offered"}`;

      const heading = document.createElement("div");
      heading.className = "handoff-heading";
      const people = document.createElement("strong");
      const receiver = handoff.to_actor_name || "pendiente de aceptación";
      people.textContent = `${handoff.from_actor_name || "Colaborador"} → ${receiver}`;
      const status = document.createElement("span");
      status.className = "handoff-status";
      status.textContent = handoffStatusLabel(handoff.status);
      heading.append(people, status);

      const summary = document.createElement("p");
      summary.className = "handoff-summary";
      summary.textContent = valueOrDash(handoff.summary);
      card.append(heading, summary);

      const details = document.createElement("div");
      details.className = "handoff-details";
      appendHandoffDetail(details, "Pendiente", handoff.remaining_work);
      appendHandoffDetail(details, "Bloqueos", handoff.blockers);
      appendHandoffDetail(details, "Siguiente", handoff.next_steps);
      const validationValues = (Array.isArray(handoff.validations) ? handoff.validations : [])
        .map((item) => `${item.name}: ${item.status}`);
      appendHandoffDetail(details, "Validaciones", validationValues);
      if (details.childElementCount > 0) card.append(details);

      const meta = document.createElement("p");
      meta.className = "handoff-meta";
      const expiry = handoff.expires_at ? ` · expira ${formatRelativeDate(handoff.expires_at)}` : "";
      meta.textContent = `Intent ${shortenIdentifier(handoff.intent_id, 8)} · v${formatInteger(handoff.version || 0)}${expiry}`;
      meta.title = handoff.intent_id || "";
      card.append(meta);
      elements.handoffList.append(card);
    }
  }

  function appendHandoffDetail(container, label, values) {
    const items = Array.isArray(values) ? values.filter(Boolean) : [];
    if (items.length === 0) return;
    const group = document.createElement("section");
    const title = document.createElement("span");
    title.textContent = label;
    const list = document.createElement("ul");
    for (const value of items.slice(0, 4)) {
      const item = document.createElement("li");
      item.textContent = value;
      list.append(item);
    }
    group.append(title, list);
    container.append(group);
  }

  function handoffStatusLabel(status) {
    return ({
      offered: "Ofrecido",
      accepted: "Aceptado",
      withdrawn: "Retirado",
      expired: "Expirado",
    })[status] || status || "Ofrecido";
  }

  function renderAttention(overview) {
    const value = overview && typeof overview === "object" ? overview : null;
    const counts = value?.counts || {};
    const activity = value?.code_activity || {};
    const repository = value?.repository_sync || {};
    elements.attentionStats.replaceChildren();
    elements.attentionList.replaceChildren();
    elements.attentionPanel.classList.remove("has-warning", "has-critical");

    if (!value) {
      elements.attentionStatus.textContent = "Consultando";
      elements.attentionDescription.textContent =
        "Esperando el estado actual del proyecto.";
      for (const label of ["En vivo", "Trabajo activo", "Bloqueos", "Cambios pendientes"]) {
        appendAttentionStat(label, "—");
      }
      return;
    }

    const liveSessions = numericCount(counts.live_sessions);
    const activeIntents = numericCount(counts.active_intents);
    const blockedIntents = numericCount(counts.blocked_intents);
    const pendingChanges = numericCount(counts.pending_changesets);
    appendAttentionStat("En vivo", liveSessions);
    appendAttentionStat("Trabajo activo", activeIntents);
    appendAttentionStat("Bloqueos", blockedIntents, blockedIntents > 0 ? "critical" : "stable");
    appendAttentionStat("Cambios pendientes", pendingChanges, pendingChanges > 0 ? "warning" : "stable");

    const issues = [];
    if (blockedIntents > 0) {
      issues.push({
        severity: "critical",
        title: `${formatInteger(blockedIntents)} ${blockedIntents === 1 ? "trabajo bloqueado" : "trabajos bloqueados"}`,
        detail: "Revisa la sección Trabajo para resolver el bloqueo o preparar un handoff.",
      });
    }
    if (!activity.observer_connected) {
      issues.push({
        severity: "warning",
        title: "No hay un observador de código conectado",
        detail: "PACT no puede confirmar modificaciones locales hasta que un nodo publique señales.",
      });
    }
    if (repository.status === "failed") {
      issues.push({
        severity: "critical",
        title: "GitHub no pudo verificar el repositorio principal",
        detail: "Abre Repositorios para revisar la conexión y volver a verificar la revisión canónica.",
      });
    }
    if (pendingChanges > 0) {
      issues.push({
        severity: "warning",
        title: `${formatInteger(pendingChanges)} ${pendingChanges === 1 ? "cambio espera integración" : "cambios esperan integración"}`,
        detail: "Hay trabajo preparado que todavía no ha completado el flujo de integración.",
      });
    }

    const criticalCount = issues.filter((issue) => issue.severity === "critical").length;
    if (criticalCount > 0) {
      elements.attentionPanel.classList.add("has-critical");
      elements.attentionStatus.textContent = `${formatInteger(criticalCount)} crítico${criticalCount === 1 ? "" : "s"}`;
      elements.attentionDescription.textContent =
        "Hay situaciones que requieren intervención antes de continuar el trabajo.";
    } else if (issues.length > 0) {
      elements.attentionPanel.classList.add("has-warning");
      elements.attentionStatus.textContent = `${formatInteger(issues.length)} por revisar`;
      elements.attentionDescription.textContent =
        "La operación continúa, pero conviene revisar estas señales.";
    } else {
      elements.attentionStatus.textContent = "Operación estable";
      elements.attentionDescription.textContent =
        "No hay bloqueos ni señales críticas en este momento.";
      issues.push({
        severity: "stable",
        title: liveSessions > 0 ? "Colaboradores conectados y sin bloqueos" : "Proyecto disponible y sin bloqueos",
        detail: liveSessions > 0
          ? "PACT está recibiendo presencia y puede coordinar nuevo trabajo."
          : "Cuando una persona o IA se conecte, su actividad aparecerá aquí.",
      });
    }

    for (const issue of issues) appendAttentionItem(issue);
    elements.tabLiveCount.textContent = formatInteger(liveSessions);
    elements.tabWorkCount.textContent = formatInteger(activeIntents);
  }

  function appendAttentionStat(label, value, severity = "neutral") {
    const item = document.createElement("div");
    item.className = `attention-stat severity-${severity}`;
    const number = document.createElement("strong");
    number.textContent = String(value);
    const copy = document.createElement("span");
    copy.textContent = label;
    item.append(number, copy);
    elements.attentionStats.append(item);
  }

  function appendAttentionItem({ severity, title, detail }) {
    const item = document.createElement("li");
    item.className = `attention-item severity-${severity}`;
    const marker = document.createElement("span");
    marker.className = "attention-marker";
    marker.setAttribute("aria-hidden", "true");
    const copy = document.createElement("div");
    const heading = document.createElement("strong");
    heading.textContent = title;
    const description = document.createElement("p");
    description.textContent = detail;
    copy.append(heading, description);
    item.append(marker, copy);
    elements.attentionList.append(item);
  }

  function numericCount(value) {
    return isNumber(value) ? Number(value) : 0;
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

  function renderProjectAccess(access) {
    const value = access && typeof access === "object" ? access : {};
    const members = Array.isArray(value.members) ? value.members : [];
    const agents = Array.isArray(value.agents) ? value.agents : [];
    const connectedAgents = agents.filter((agent) => agent.connected);

    elements.accessMemberCount.textContent = formatInteger(members.length);
    elements.accessAgentCount.textContent = formatInteger(agents.length);
    elements.accessConnectedCount.textContent = formatInteger(connectedAgents.length);
    elements.accessMemberListCount.textContent = formatInteger(members.length);
    elements.accessAgentListCount.textContent = formatInteger(agents.length);
    elements.tabAccessCount.textContent = formatInteger(members.length);
    elements.accessMemberEmpty.hidden = members.length !== 0;
    elements.accessAgentEmpty.hidden = agents.length !== 0;
    elements.accessLiveStatus.classList.toggle("is-live", connectedAgents.length > 0);
    elements.accessLiveStatus.lastChild.textContent =
      ` ${formatInteger(connectedAgents.length)} conectados`;

    elements.accessMemberList.replaceChildren();
    for (const member of members) {
      const row = document.createElement("article");
      row.className = "access-entry access-member-entry";
      if (member.status !== "active") row.classList.add("is-disabled");

      const identity = document.createElement("div");
      identity.className = "access-identity";
      const avatar = document.createElement("span");
      avatar.className = "actor-avatar";
      avatar.textContent = initials(member.display_name || member.principal_id);
      avatar.setAttribute("aria-hidden", "true");
      const copy = document.createElement("div");
      copy.className = "access-identity-copy";
      const nameLine = document.createElement("div");
      nameLine.className = "access-name-line";
      const name = document.createElement("strong");
      name.textContent = valueOrDash(member.display_name || member.principal_id);
      nameLine.append(name);
      if (member.principal_id === state.principal?.id) {
        const current = document.createElement("span");
        current.className = "current-user-chip";
        current.textContent = "Tú";
        nameLine.append(current);
      }
      const kind = document.createElement("span");
      kind.textContent = member.bootstrap
        ? "Administrador local de PACT"
        : member.principal_type === "service" ? "Cuenta de servicio" : "Persona";
      copy.append(nameLine, kind);
      identity.append(avatar, copy);

      const permission = document.createElement("div");
      permission.className = "access-permission";
      const role = document.createElement("span");
      role.className = `access-role role-${member.effective_role || "viewer"}`;
      role.textContent = projectRoleCopy[member.effective_role] || valueOrDash(member.effective_role);
      const source = document.createElement("span");
      source.textContent = accessSourceCopy[member.access_source] || valueOrDash(member.access_source);
      permission.append(role, source);

      row.append(identity, permission);
      elements.accessMemberList.append(row);
    }

    elements.accessAgentList.replaceChildren();
    for (const agent of agents) {
      const row = document.createElement("article");
      row.className = "access-entry access-agent-entry";
      row.classList.toggle("is-connected", Boolean(agent.connected));
      row.classList.toggle("is-disabled", !agent.access_active);

      const identity = document.createElement("div");
      identity.className = "access-identity";
      const presence = document.createElement("span");
      presence.className = "agent-presence";
      presence.setAttribute("aria-hidden", "true");
      const copy = document.createElement("div");
      copy.className = "access-identity-copy";
      const name = document.createElement("strong");
      name.textContent = valueOrDash(agent.display_name || agent.agent_id);
      const kind = document.createElement("span");
      kind.textContent = [agent.agent_type, agent.last_client_type]
        .filter((item, index, list) => item && list.indexOf(item) === index)
        .join(" · ") || "Agente";
      copy.append(name, kind);
      identity.append(presence, copy);

      const ownership = document.createElement("div");
      ownership.className = "agent-ownership";
      const ownerLabel = document.createElement("span");
      ownerLabel.textContent = "Responsable";
      const owner = document.createElement("strong");
      owner.textContent = valueOrDash(agent.sponsor_display_name);
      const ownerRole = document.createElement("span");
      ownerRole.textContent = agent.access_active
        ? projectRoleCopy[agent.sponsor_effective_role] || valueOrDash(agent.sponsor_effective_role)
        : "Sin acceso vigente";
      ownership.append(ownerLabel, owner, ownerRole);

      const session = document.createElement("div");
      session.className = "agent-session-state";
      const connection = document.createElement("strong");
      connection.textContent = agent.connected
        ? `${formatInteger(agent.active_sessions)} ${Number(agent.active_sessions) === 1 ? "sesión activa" : "sesiones activas"}`
        : "Desconectado";
      const lastSeen = document.createElement("time");
      lastSeen.textContent = agent.last_seen_at
        ? `Última señal ${formatRelativeDate(agent.last_seen_at)}`
        : "Sin señales registradas";
      if (agent.last_seen_at) {
        lastSeen.dateTime = agent.last_seen_at;
        lastSeen.title = formatDateTime(agent.last_seen_at);
      }
      const node = document.createElement("span");
      node.textContent = agent.last_node_name
        ? `Nodo: ${agent.last_node_name}`
        : `${formatInteger(agent.session_count)} sesiones históricas`;
      session.append(connection, lastSeen, node);

      row.append(identity, ownership, session);
      elements.accessAgentList.append(row);
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
    elements.tabActivityCount.textContent = formatInteger(events.length);

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
      const headline = document.createElement("div");
      headline.className = "event-headline";
      const type = document.createElement("strong");
      type.className = "event-type";
      type.textContent = eventNarrative(event);
      type.title = event.type || "";
      const kind = document.createElement("span");
      kind.className = "event-kind";
      kind.textContent = eventTypeCopy[event.type] || "Evento de PACT";
      headline.append(type, kind);
      const time = document.createElement("time");
      time.className = "event-time";
      time.textContent = event.occurred_at
        ? formatClock(event.occurred_at)
        : "—";
      if (event.occurred_at) {
        time.dateTime = event.occurred_at;
        time.title = formatDateTime(event.occurred_at);
      }
      titleRow.append(headline, time);

      const meta = document.createElement("div");
      meta.className = "event-meta";
      const eventData = eventDataObject(event.data);
      appendEventTextMeta(
        meta,
        "Repositorio",
        eventData.repository_full_name || eventData.repository_sync?.repository_full_name,
      );
      appendEventTextMeta(meta, "Rama", eventData.branch || eventData.git_branch);
      appendEventTextMeta(
        meta,
        "Archivos",
        isNumber(eventData.changed_paths)
          ? formatInteger(eventData.changed_paths)
          : null,
      );
      appendEventMeta(meta, "Sesión", event.session_id);
      appendEventMeta(meta, "Trabajo", event.intent_id);

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

  function appendEventTextMeta(container, label, value) {
    if (value === null || value === undefined || value === "") return;
    const span = document.createElement("span");
    const strong = document.createElement("strong");
    strong.textContent = String(value);
    span.append(`${label} `, strong);
    container.append(span);
  }

  function eventNarrative(event) {
    const data = eventDataObject(event.data);
    const actor = event.actor_name || "PACT";
    const title = data.title || data.intent?.title || "un trabajo";
    const project = data.name || data.project?.name || "el proyecto";
    const handoff = data.handoff?.summary || data.summary || "el relevo de trabajo";
    const knowledge =
      data.record?.title ||
      data.resource?.title ||
      data.resource?.name ||
      "nuevo conocimiento";
    const repository =
      data.repository_full_name ||
      data.repository_sync?.repository_full_name ||
      "el repositorio";
    const changedPaths = numericCount(data.changed_paths);
    const changeCopy = `${formatInteger(changedPaths)} ${changedPaths === 1 ? "archivo" : "archivos"}`;

    const narratives = {
      "pact.project.created.v1": () => `Se creó ${project}`,
      "pact.intent.started.v1": () => `${actor} comenzó “${title}”`,
      "pact.intent.active.v1": () => `${actor} reanudó “${title}”`,
      "pact.intent.blocked.v1": () => `${actor} bloqueó “${title}”`,
      "pact.intent.submitted.v1": () => `${actor} envió “${title}” a revisión`,
      "pact.intent.completed.v1": () => `${actor} completó “${title}”`,
      "pact.intent.cancelled.v1": () => `${actor} canceló “${title}”`,
      "pact.intent.abandoned.v1": () => `${actor} abandonó “${title}”`,
      "pact.workspace.ready.v1": () => `${actor} preparó un worktree para “${title}”`,
      "pact.workspace.diff_updated.v1": () =>
        changedPaths > 0
          ? `${actor} modificó ${changeCopy}`
          : `${actor} actualizó el estado del código`,
      "pact.workspace.head_updated.v1": () => `${actor} avanzó la revisión del worktree`,
      "pact.git.external_change_detected.v1": () =>
        `${actor} produjo un cambio fuera del flujo coordinado`,
      "pact.session.started.v1": () => `${actor} se conectó al proyecto`,
      "pact.session.closed.v1": () => `${actor} cerró su sesión`,
      "pact.repository.canonical_synced.v1": () => `${actor} verificó ${repository} con GitHub`,
      "pact.repository.sync_failed.v1": () => `PACT no pudo verificar ${repository}`,
      "pact.project.repository_attached.v1": () => `${actor} vinculó ${repository} al proyecto`,
      "pact.repository-observation.v1": () =>
        changedPaths > 0
          ? `${actor} observó cambios en ${changeCopy}`
          : `${actor} comprobó el estado del código`,
      "pact.changeset.created.v1": () => `${actor} preparó un conjunto de cambios`,
      "pact.handoff.offered.v1": () => `${actor} ofreció “${handoff}”`,
      "pact.handoff.accepted.v1": () => `${actor} aceptó “${handoff}”`,
      "pact.handoff.expired.v1": () => `Venció “${handoff}”`,
      "pact.context.compiled.v1": () => `${actor} compiló un nuevo paquete de contexto`,
      "pact.knowledge.record.proposed.v1": () => `${actor} propuso “${knowledge}”`,
      "pact.knowledge.resource.added.v1": () => `${actor} añadió “${knowledge}”`,
    };
    return narratives[event.type]?.() ||
      `${actor} registró ${eventTypeCopy[event.type] || "un evento del proyecto"}`;
  }

  function eventDataObject(value) {
    if (value && typeof value === "object") return value;
    if (typeof value !== "string") return {};
    try {
      const parsed = JSON.parse(value);
      return parsed && typeof parsed === "object" ? parsed : {};
    } catch {
      return {};
    }
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
    clearInterval(state.roomPollTimer);
    state.roomPollTimer = null;
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

  async function requestJSON(
    path,
    { token = state.token, method = "GET", headers = {}, body } = {},
  ) {
    const requestHeaders = {
      Accept: "application/json",
      Authorization: `Bearer ${token}`,
      ...headers,
    };
    if (body !== undefined) requestHeaders["Content-Type"] = "application/json";
    const response = await fetch(path, {
      method,
      headers: requestHeaders,
      body: body === undefined ? undefined : JSON.stringify(body),
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
    state.workspaces = [];
    state.projects = [];
    state.selectedWorkspaceID = null;
    state.selectedProjectID = null;
    state.workspaceSection = "overview";
    state.workspaceRoomDirectories.clear();
    state.workspaceRoomDirectoryLoading.clear();
    state.workspaceContext = null;
    resetWorkspaceRooms();
    state.globalRoomMentions = [];
    renderGlobalRoomMentionCount();
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
