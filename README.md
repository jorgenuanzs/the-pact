# The Pact

Pact es un plano de control para proyectos en los que personas y agentes de IA
comparten conocimiento, coordinan trabajo y realizan acciones verificables
sobre código, infraestructura y otros recursos.

El objetivo no es sustituir Git. Git conserva el contenido y el historial del
código; Pact mantiene el estado operativo vivo alrededor de ese contenido:
quién actúa, qué intenta conseguir, qué recursos afecta, qué ocurrió y qué
contexto necesita el siguiente participante.

## Estado

El primer vertical técnico ya tiene una base ejecutable:

- Pact Server como monolito modular en Go;
- PostgreSQL 18 con pgvector como persistencia canónica;
- migraciones SQL embebidas y verificadas mediante checksum;
- API HTTP local autenticada;
- creación idempotente de proyectos;
- estado, evento y outbox confirmados en una sola transacción;
- recuperación de eventos por cursor y stream SSE reanudable;
- entorno reproducible mediante Docker Compose.

El siguiente corte incorporará identidades, sesiones, intenciones y scopes;
después llegará Pact Node para operar Git y worktrees desde el host.

## Inicio rápido

Solo se requieren Docker, Docker Compose y Make:

```sh
make init
make dev
```

Comprueba el servidor:

```sh
curl --fail-with-body http://127.0.0.1:8080/livez
curl --fail-with-body http://127.0.0.1:8080/readyz
```

La guía de [desarrollo local](docs/development.md) contiene el flujo completo
para crear un proyecto, recuperar eventos, ejecutar pruebas y diagnosticar el
entorno.

## Arquitectura del primer corte

```text
Agente o cliente
      │ HTTP + JSON / SSE
      ▼
  Pact Server
      │ transacciones SQL
      ▼
PostgreSQL + pgvector

Pact Node (siguiente corte) ── Git + worktrees en el host
```

Pact Server no monta el repositorio del usuario ni el socket de Docker. La base
de datos permanece en una red privada y la API se publica únicamente en
loopback durante el desarrollo local.

## Documentación

- [Documento maestro](PACT_MASTER_PLAN.md)
- [ADR-0001: primer recorrido de producto](docs/adr/0001-first-product-slice.md)
- [ADR-0002: fundación de plataforma](docs/adr/0002-platform-foundation.md)
- [Especificación del bucle central v0.1](docs/spec/core-loop-v0.1.md)
- [Contrato OpenAPI](api/openapi.yaml)
- [Desarrollo local](docs/development.md)

## Primer objetivo de producto

Demostrar que dos agentes pueden trabajar sobre el mismo repositorio, compartir
estado útil y coordinar cambios sin tener que intercambiar sus conversaciones.
