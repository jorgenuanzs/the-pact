# ADR-0011 — Workspaces compartidos y vocabulario de worktrees

**Estado:** aceptado
**Fecha:** 14 de agosto de 2026

## Contexto

PACT utilizaba `workspace` para nombrar el checkout Git aislado que pertenece a
una intención. Ese significado técnico impedía utilizar el mismo término para
el espacio durable donde un equipo comparte varios proyectos, miembros,
recursos y conocimiento.

Un proyecto tampoco es suficiente como frontera superior: un producto puede
tener proyectos separados para web, API e infraestructura que necesitan el
mismo contexto y las mismas decisiones.

## Decisión

El modelo de dominio adopta esta jerarquía:

```text
Organization → Workspace → Project → Intent → Worktree
```

- `Workspace` es la frontera durable de colaboración y contexto.
- Un Workspace contiene cero o más proyectos.
- Un proyecto pertenece a un único Workspace para evitar contextos ambiguos.
- `Worktree` es el checkout Git aislado asociado a una intención y sesión.
- Cada proyecto existente recibe un Workspace predeterminado durante la
  migración.
- Crear un proyecto crea también su Workspace predeterminado en la misma
  transacción.
- Mover un proyecto entre Workspaces no modifica Git ni el checkout local.

La tabla técnica `coordination.workspaces` se renombra a
`coordination.worktrees`. El endpoint v0.7
`POST /v1/intents/{intent_id}/workspace` permanece temporalmente disponible y
se marca como obsoleto. Los clientes nuevos usan `/worktree`.

Los permisos de este primer corte se derivan de los proyectos visibles. Los
administradores de la organización pueden crear Workspaces y mover proyectos.
La tabla `workspace_members` prepara permisos explícitos posteriores sin
ampliar todavía la superficie pública.

## Primer corte

Workspace Foundation incluye:

- persistencia de Workspaces, proyectos asociados y miembros futuros;
- creación, listado, lectura y asociación de proyectos mediante HTTP;
- comandos CLI para administrar Workspaces;
- agrupación de proyectos por Workspace en el backoffice;
- migración automática para instalaciones existentes;
- compatibilidad HTTP con el vocabulario anterior de worktree.

Records, Resources, Handoffs, búsqueda y compilación de contexto se construirán
sobre esta frontera. No forman parte de esta decisión inicial.

## Consecuencias

- El vocabulario distingue contexto compartido de aislamiento Git.
- Un producto puede reunir varios repositorios y proyectos sin fusionarlos.
- Las instalaciones actuales conservan sus proyectos y clientes v0.7.
- Durante la transición, algunos nombres de eventos y campos JSON mantienen
  `workspace` por compatibilidad y deberán versionarse antes de v1.
- Los Workspaces vacíos solo son visibles para administradores hasta que haya
  membresías explícitas.
