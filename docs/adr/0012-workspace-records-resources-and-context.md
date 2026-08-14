# ADR-0012 — Records, Resources y contexto determinista de Workspace

**Estado:** aceptado
**Fecha:** 14 de agosto de 2026

## Contexto

Workspace Foundation creó la frontera durable que agrupa proyectos, pero no
existía todavía una representación canónica para decisiones, requisitos,
restricciones, riesgos ni fuentes. Guardar estos hechos únicamente en chats o
archivos Markdown obliga a cada participante a reconstruir su vigencia y su
procedencia.

PACT necesita aportar memoria útil antes de incorporar ingestión documental,
embeddings o síntesis mediante IA. Esa memoria debe ser explícita, autorizada,
reproducible y capaz de distinguir una propuesta de un hecho aceptado.

## Decisión

Se incorporan dos conceptos dentro de cada Workspace:

- `Resource`: referencia a una fuente externa o interna. Conserva tipo, título,
  localizador, clasificación y metadatos, pero no copia su contenido.
- `Record`: afirmación durable y tipada, como decisión, requisito, restricción,
  supuesto, riesgo, pregunta, hallazgo, procedimiento, incidente o resultado de
  validación.

Un Record comienza como `proposed`. Su ciclo de vida distingue `accepted`,
`disputed`, `superseded`, `revoked`, `expired` y `rejected`. Las transiciones
usan control optimista de versión. Solo un maintainer puede revisar estados;
un contributor puede registrar fuentes y proponer conocimiento.

La tabla de evidencia relaciona Records y Resources mediante `supports`,
`contradicts`, `origin` o `validates`. Toda mutación usa una clave idempotente y
añade un evento inmutable en `knowledge.events` con hash del payload.

El compilador inicial de contexto es determinista:

- incluye decisiones, requisitos y restricciones aceptados o disputados;
- incluye preguntas abiertas y riesgos todavía activos aunque estén propuestos;
- excluye registros rechazados, revocados, expirados o reemplazados;
- devuelve fuentes activas y advertencias sobre conocimiento disputado;
- no invoca un LLM ni requiere embeddings.

PostgreSQL mantiene búsqueda de texto completo con `tsvector`. pgvector sigue
disponible en la plataforma, pero no se utiliza hasta que exista contenido
ingerido, una política de embeddings y evaluaciones de recuperación.

Los localizadores de Resources no pueden contener credenciales incrustadas ni
parámetros de consulta que aparenten guardar tokens, secretos, contraseñas,
firmas o claves de API. La clasificación no reemplaza todavía políticas de
acceso por registro; todos los miembros autorizados del Workspace reciben las
fuentes que pueden leer mediante esta primera API.

## Superficies

HTTP expone creación y búsqueda de Resources, propuesta y revisión de Records,
lectura de evidencia y compilación de contexto por Workspace.

MCP añade:

```text
pact.workspace_context
pact.list_resources
pact.add_resource
pact.list_records
pact.propose_record
pact.review_record
```

`pact.project_context` incorpora también el contexto del Workspace conectado.
El backoffice muestra una proyección de solo lectura con acuerdos vigentes,
asuntos que requieren atención y fuentes registradas.

## Consecuencias

- Diferentes proyectos y agentes pueden compartir hechos sin intercambiar sus
  conversaciones completas.
- Una decisión propuesta no se presenta como autoridad aceptada.
- La evidencia queda navegable y auditable desde el primer corte.
- La búsqueda semántica futura podrá añadirse sobre entidades estables sin
  cambiar el contrato conceptual.
- Todavía no existe ingestión del contenido, extracción automática, permisos
  por clasificación, handoffs, Context Packs por intención ni recuperación
  híbrida con embeddings.

ADR-0013 incorpora posteriormente Handoffs y Context Packs sin modificar el
modelo de Records, Resources ni la decisión de posponer embeddings.
