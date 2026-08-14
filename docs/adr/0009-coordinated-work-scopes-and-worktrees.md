# ADR-0009 — Trabajo coordinado, scopes y worktrees aislados

**Estado:** aceptado
**Fecha:** 13 de agosto de 2026

## Contexto

Una sesión viva indica quién está conectado y la observación Git indica que un
checkout cambió, pero ninguna de las dos responde por sí sola qué resultado
busca el actor ni qué parte del repositorio pretende modificar. Dos agentes
pueden recibir el mismo objetivo, editar rutas solapadas y descubrir el
conflicto demasiado tarde.

Git worktree permite aislar directorios y ramas sin duplicar el repositorio.
No resuelve la asignación del trabajo: PACT necesita declarar la intención y
reservar recursos antes de crear el worktree.

## Decisión

PACT incorpora un bucle coordinado único para clientes MCP y HTTP:

```text
contexto → comprobar scopes → iniciar intención y leases
         → crear worktree local → registrar worktree
         → observar cambios → actualizar estado y resumen
```

### Intención

Una intención activa guarda título, objetivo, criterios de éxito, revisión Git
base, agente responsable, estado, versión y resumen durable. Sus transiciones
utilizan control optimista mediante `expected_version`.

### Scopes y exclusión

El cliente declara entre uno y cincuenta scopes `repository`, `path` o `file`.
PACT normaliza rutas relativas y rechaza rutas absolutas, `..` y cualquier
referencia que escape del repositorio.

El modo predeterminado es `exclusive`. Dos claims `shared` pueden solaparse; un
claim exclusivo bloquea cualquier scope jerárquicamente solapado. Por ejemplo,
`path:internal` se solapa con `file:internal/api.go`, pero no con
`file:internal2/api.go`.

La operación de inicio toma un advisory lock transaccional por proyecto antes
de comprobar y crear claims. Esto impide la carrera en la que dos comandos
simultáneos observan el scope libre y ambos lo reservan. `allow_overlap=true`
permite una anulación explícita; el solapamiento y la anulación quedan en el
evento durable.

Los claims son leases de sesenta segundos asociados a la sesión. Cada
heartbeat los renueva y cerrar la sesión los libera. Los claims expirados no
bloquean trabajo nuevo. Completar, cancelar o abandonar una intención también
los libera.

### Worktree local

Después de crear la intención, el adaptador local crea un Git worktree real en:

```text
.pact/worktrees/<intent-id>
```

La rama tiene la forma `pact/<id-corto>-<título>`. El worktree parte del commit
exactamente registrado en la intención y copia únicamente la configuración
local reconstruible de PACT. `.pact/` ya está ignorado por Git. PACT rechaza un
symlink o un directorio que pertenezca a otro trabajo en la ruta administrada.

Pact Server solo guarda `path_ref`, una referencia relativa. La ruta absoluta
se devuelve exclusivamente al agente local al que se asignó el worktree y no
aparece en el contexto compartido de otros agentes.

### Observación y estados

Cada worktree tiene su propia observación Git, separada de la observación del
checkout principal. Un diff emite `pact.workspace.diff_updated.v1`; avanzar el
HEAD dentro del worktree emite `pact.workspace.head_updated.v1`. Un cambio de
HEAD en el checkout no gestionado conserva
`pact.git.external_change_detected.v1`.

Enviar una intención congela su worktree. Completar, cancelar o abandonar la
intención archiva el worktree. Todas las transiciones escriben evento y outbox
en la misma transacción que el estado.

### Contrato para agentes

El servidor MCP añade:

```text
pact.check_scopes
pact.start_work
pact.list_work
pact.update_work
```

Sus instrucciones exigen consultar contexto, comprobar scopes, iniciar trabajo
y editar únicamente `workspace_path`. HTTP expone las mismas operaciones para
otros adaptadores. PACT no guarda conversaciones privadas; guarda el objetivo,
la asignación, el estado y el resumen que otros participantes necesitan.

### Backoffice

La vista del proyecto combina dos niveles:

- presencia: sesiones y nodos que responden ahora;
- trabajo: intención, responsable, objetivo o resumen, scopes, rama, revisión
  base, worktree, estado y última señal.

Los eventos se actualizan por SSE y una lectura del overview se programa tras
cada evento. El panel no infiere “trabajando” solo porque una sesión exista:
la marca viva del tablero requiere una sesión reciente y una intención activa o
bloqueada.

## Frontera de garantía

PACT coordina y aísla el flujo que pasa por sus herramientas, pero no es un
sandbox del sistema operativo. Un proceso con permisos normales todavía puede
ignorar el protocolo y editar el checkout principal o ejecutar Git
directamente. Pact Node lo observa y registra, pero no puede impedirlo.

La garantía estricta de escritura requerirá runners o sandboxes que entreguen
al agente únicamente el worktree autorizado. Este corte establece el contrato
y el aislamiento local necesarios para añadir esa ejecución restringida sin
cambiar el modelo de dominio.

## Consecuencias

- Dos agentes compatibles conocen el solapamiento antes de escribir.
- El trabajo concurrente comienza en ramas y directorios separados.
- Un dashboard puede responder quién trabaja, qué busca, dónde modifica y qué
  ocurrió sin leer sus conversaciones.
- La pérdida abrupta de un agente no deja un bloqueo permanente.
- El override explícito conserva flexibilidad, pero queda auditable.
- Integración automática, ChangeSets inmutables y limpieza de worktrees
  terminados siguen siendo cortes posteriores.
