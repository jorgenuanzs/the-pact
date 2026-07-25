# Especificación del bucle central de Pact v0.1

**Estado:** borrador  
**Depende de:** ADR-0001  
**Objetivo:** convertir el primer recorrido de producto en contratos implementables.

## 1. Resultado observable

Pact v0.1 debe permitir que dos agentes operen sobre un repositorio local con:

- identidad y sesión separadas;
- intención explícita;
- revisión base conocida;
- workspace aislado;
- estado compartido;
- avisos por solapamiento;
- presentación inmutable;
- validación;
- integración;
- recuperación desde eventos.

## 2. Componentes

```text
pact CLI
  Inicia, configura y consulta.

Pact Server
  Autoridad de estado, eventos y coordinación.

PostgreSQL + pgvector
  Persistencia canónica. pgvector queda habilitado aunque no sea necesario
  para recuperar el primer ContextPacket.

Pact Node
  Observa el repositorio y administra worktrees.

Agent adapter
  Traduce herramientas de un chat o agente a comandos del protocolo.
```

## 3. Entidades mínimas

### Project

```text
id
name
root_repository_id
canonical_revision
created_at
version
```

### Principal

Identidad responsable.

```text
id
type: human | service
display_name
```

### Agent

Identidad lógica de un agente. No representa una conexión.

```text
id
sponsor_principal_id
name
agent_type
capabilities
```

### Session

Presencia temporal de una instancia de agente.

```text
id
agent_id
project_id
node_id
status
started_at
last_seen_at
expires_at
```

Estados:

```text
starting → active → stale → closed | expired
```

### Intent

Objetivo duradero.

```text
id
project_id
title
goal
success_criteria
status
base_revision
responsible_agent_id
created_at
version
```

Estados iniciales:

```text
draft → active ↔ blocked → submitted → completed
                               └──────→ cancelled | abandoned
```

### ResourceRef

Referencia uniforme a algo que puede verse afectado.

```text
kind: repository | path | file | component | symbol
locator
revision
```

### ScopeClaim

```text
id
intent_id
resource_ref
origin: declared | observed | inferred
confidence
evidence
status
```

v0.1 implementará:

- `declared`;
- `observed` a partir del diff.

`inferred` se reserva para análisis posterior.

### Workspace

```text
id
project_id
intent_id
session_id
base_revision
path_ref
git_branch
status
created_at
```

Estados:

```text
provisioning → ready → active → frozen → archived
```

### ChangeSet

```text
id
workspace_id
intent_id
base_revision
content_hash
git_tree
patch_object_ref
status
created_at
```

Un ChangeSet es inmutable. Si el contenido cambia, se crea otro.

### ValidationRun

```text
id
changeset_id
changeset_hash
validation_type
status
started_at
finished_at
result
```

### IntegrationAttempt

```text
id
changeset_id
target_revision
status
result_revision
created_at
```

### Event

Registro durable de una transición confirmada.

### ContextPacket

Vista reproducible para una sesión e intención.

## 4. Comandos

```text
project.create
project.open

agent.register

session.start
session.heartbeat
session.close

intent.create
intent.activate
intent.block
intent.submit
intent.complete
intent.cancel

scope.declare

workspace.create
workspace.freeze
workspace.archive

changeset.create
validation.request
integration.request

context.compile
```

Cada comando mutable exige:

- `command_id`;
- `idempotency_key`;
- `project_id`;
- `actor_id`;
- `session_id` cuando aplique;
- `expected_version` cuando modifica un agregado;
- `correlation_id`;
- payload validado.

## 5. Consultas

```text
project.get
project.status

session.list_active

intent.get
intent.list

scope.list
scope.overlaps

workspace.get

changeset.get

event.list_after_cursor

context.get
```

## 6. Eventos

```text
project.created
agent.registered

session.started
session.became_stale
session.closed
session.expired

intent.created
intent.activated
intent.blocked
intent.submitted
intent.completed
intent.cancelled

scope.declared
scope.observed
overlap.detected

workspace.provisioned
workspace.diff_updated
workspace.frozen
workspace.archived

changeset.created
validation.started
validation.passed
validation.failed

integration.started
integration.conflicted
integration.succeeded
integration.failed

git.external_change_detected
project.canonical_revision_changed
context.invalidated
```

Los heartbeats actualizan `last_seen_at`; no generan un evento durable cada vez.

## 7. ContextPacket v0.1

```json
{
  "project": {
    "id": "prj_01",
    "canonical_revision": "81af23"
  },
  "snapshot": {
    "event_cursor": 481,
    "generated_at": "2026-07-25T12:00:00Z"
  },
  "requesting_session": "ses_02",
  "intent": {},
  "active_intents": [],
  "active_sessions": [],
  "declared_scopes": [],
  "observed_scopes": [],
  "overlaps": [],
  "recent_changes": [],
  "warnings": [],
  "expires_at": "2026-07-25T12:05:00Z"
}
```

Advertencias iniciales:

```text
BASE_REVISION_STALE
OVERLAPPING_SCOPE
SESSION_STALE
EXTERNAL_GIT_CHANGE
VALIDATION_FAILED
CONTEXT_EXPIRED
```

## 8. Flujo de workspace

```text
1. Pact Server autoriza la creación.
2. Pact Node recibe el trabajo.
3. Verifica que el repositorio esté limpio desde la perspectiva de Pact.
4. Crea un worktree desde el hash exacto.
5. Publica workspace.provisioned.
6. El agente recibe la ruta autorizada.
7. Node observa cambios con debounce.
8. Calcula rutas modificadas.
9. Publica scope.observed y workspace.diff_updated.
10. Al presentar, congela el workspace.
11. Calcula árbol, patch y hash.
12. Crea ChangeSet.
```

El worktree debe vivir fuera del directorio principal del usuario para evitar mezclar cambios.

## 9. Detección de solapamientos v0.1

v0.1 comparará:

- repositorio completo;
- prefijos de ruta;
- archivos exactos.

Reglas:

```text
file == file                   → overlap exacto
path contiene file            → overlap por ruta
path A contiene path B        → overlap jerárquico
repository                    → overlap global
```

No bloqueará la edición. Publicará:

- recursos;
- intenciones;
- sesiones;
- origen de cada claim;
- gravedad inicial;
- recomendación.

## 10. Integración v0.1

```text
1. Recibir ChangeSet inmutable.
2. Comprobar hash.
3. Obtener revisión canónica actual.
4. Crear integración especulativa.
5. Ejecutar validaciones configuradas.
6. Si la base cambió, intentar aplicar o marcar conflicto.
7. Actualizar la ref local canónica.
8. Confirmar el commit resultante.
9. Registrar integration.succeeded.
10. Cambiar canonical_revision.
11. Invalidar ContextPackets anteriores.
```

Todavía no se prometerá control de una referencia remota.

## 11. Persistencia inicial

Tablas:

```text
projects
repositories
principals
agents
nodes
sessions
intents
resource_refs
scope_claims
workspaces
changesets
validation_runs
integration_attempts
events
outbox
idempotency_records
```

Una transacción de comando escribe:

1. estado;
2. evento;
3. outbox;
4. resultado idempotente.

## 12. Transportes iniciales

Propuesta:

- HTTP + JSON para comandos y consultas;
- SSE para eventos recuperables por cursor;
- WebSocket para el canal persistente Pact Node–Server;
- socket local o HTTP loopback para agent adapters.

## 13. Seguridad inicial

- servidor escuchando en loopback por defecto;
- token local de proyecto;
- un agent token por sesión;
- scopes limitados al proyecto;
- rutas de workspace asignadas;
- Node valida toda ruta;
- no ejecutar shell arbitrario enviado por un agente;
- ningún secreto en eventos;
- PostgreSQL no expuesto fuera de la red de Compose;
- logs estructurados y filtrados.

## 14. Pruebas de aceptación

### Dos agentes

1. Registrar principal y dos agentes.
2. Abrir dos sesiones.
3. Crear dos intenciones desde el mismo commit.
4. Declarar scopes solapados.
5. Comprobar `overlap.detected`.
6. Consultar contexto desde ambas sesiones.
7. Crear dos worktrees diferentes.

### ChangeSet

1. Modificar un archivo.
2. Observar el scope.
3. Congelar.
4. Crear ChangeSet.
5. Validar.
6. Integrar.
7. Confirmar la revisión nueva.
8. Invalidar el contexto anterior.

### Cambio externo

1. Crear una revisión Git fuera de Pact.
2. Detectarla.
3. No atribuirla a un agente.
4. Actualizar revisión.
5. Advertir a las intenciones anteriores.

### Recuperación

1. Crear proyecto, sesiones e intenciones.
2. Detener Pact.
3. Reiniciar.
4. Recuperar eventos y estado.
5. Expirar sesiones antiguas.
6. Conservar intenciones y ChangeSets.

## 15. Fuera de v0.1, pero protegido por diseño

- vector retrieval;
- LLM summaries;
- symbol graph;
- documentos;
- OIDC;
- organizaciones;
- policy engine avanzado;
- secret broker;
- infra adapters;
- runners remotos;
- multi-repository snapshots;
- UI completa.

