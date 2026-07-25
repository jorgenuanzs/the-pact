# ADR-0002 — Fundación de plataforma

**Estado:** aceptado
**Fecha:** 25 de julio de 2026
**Decisión que implementa:** base técnica sobre la que crecerá el primer bucle
central de Pact.

## Contexto

ADR-0001 define el recorrido que debe probar Pact, pero no fija cómo conservar
sus garantías cuando hay concurrencia, reintentos, reinicios o varios tipos de
actor. Antes de añadir sesiones, intenciones y Git, la plataforma necesita
demostrar un comando mutable completo y recuperable.

La fundación debe servir al modo local sin crear un modelo desechable que luego
obligue a reescribir todas las claves para soportar equipos.

## Decisión

### Aplicación

Pact Server será inicialmente un monolito modular en Go:

- un único módulo;
- biblioteca estándar para HTTP, JSON, logs, señales y lifecycle;
- `pgx/v5` para PostgreSQL;
- SQL explícito, sin ORM ni framework HTTP;
- paquetes agrupados por responsabilidad de dominio y plataforma.

Se añadirá un binario separado para Pact Node cuando exista un protocolo real
de conexión y trabajo. Pact Server no ejecutará Git ni recibirá un montaje del
repositorio del usuario.

### Persistencia

PostgreSQL 18 será la persistencia canónica desde el comienzo. La extensión
pgvector queda habilitada en la primera migración, aunque el primer
`ContextPacket` todavía no utilice embeddings.

El esquema contiene `organization_id` y relaciones tenant-safe desde la primera
versión. El modo local utiliza una organización sembrada e implícita para la
API; esto no equivale todavía a ofrecer multi-tenancy ni autorización para
equipos.

Los identificadores se almacenan como UUID y PostgreSQL 18 genera UUIDv7. Los
schemas físicos iniciales son:

```text
identity
coordination
platform
```

Las migraciones están embebidas en el binario, se ejecutan dentro de una
transacción, toman un advisory lock y registran su checksum. El servidor no
queda disponible si su migración conocida no está aplicada con el checksum
esperado.

### Identidad

`actors` es el supertipo durable de principals, agents, nodes y futuros
runners/connectors. Esto permite que un evento tenga una clave foránea real para
su actor sin recurrir a una referencia polimórfica imposible de validar.

La identidad estable de un actor y la presencia temporal de una sesión son
conceptos separados. Un comando autenticado derivará el actor desde sus
credenciales; no confiará en un `actor_id` arbitrario enviado en el cuerpo.

### Comandos y concurrencia

Toda mutación seguirá una unidad transaccional:

```text
idempotencia
    → cambio de estado
    → evento durable
    → outbox
    → resultado reutilizable
    → commit
```

Reutilizar una clave de idempotencia con el mismo comando y payload devuelve el
resultado almacenado. Reutilizarla con un payload diferente produce un
conflicto y no genera otro efecto.

`expected_version` se reservará para concurrencia optimista de un agregado.
`base_revision` y `target_revision` se reservarán para revisiones Git; no son
intercambiables.

`project.create` es un comando de bootstrap: exige organización, pero todavía
no puede exigir `project_id`.

### Eventos y entrega

Cada proyecto posee un contador transaccional. Su `project_sequence` define el
orden recuperable público y evita que un consumidor salte un evento por el
orden diferente en que dos transacciones reservaron IDs y confirmaron.

El cursor se transporta como string opaco, aunque v0.1 utilice internamente un
`bigint`.

La tabla `events` es append-only. El outbox se confirma en la misma transacción
y permitirá entrega al menos una vez. SSE recupera siempre desde eventos
durables por cursor; una notificación futura podrá reducir latencia, pero nunca
será la fuente de verdad.

### Transporte y entorno local

El primer transporte es HTTP + JSON, con SSE para eventos:

- `/livez` comprueba solo que el proceso responde;
- `/readyz` confirma que el servidor terminó de iniciar y PostgreSQL responde;
- las operaciones se autentican con un token local;
- los cuerpos JSON son estrictos y limitados;
- los errores utilizan `application/problem+json`;
- la API escucha solo en loopback en el entorno local.

Docker Compose inicia PostgreSQL, una tarea de migración y Pact Server. La base
de datos no publica un puerto en el host. El runtime es no-root, read-only, sin
capacidades Linux y sin acceso al socket de Docker.

## Capacidades deliberadamente posteriores

- RLS y memberships de organizaciones;
- roles PostgreSQL separados para owner, migrator, runtime y workers;
- autenticación OIDC y tokens de sesión;
- worker de publicación del outbox;
- particionado de eventos;
- Redis, Kafka o un broker externo;
- RAG, embeddings y síntesis mediante LLM;
- Pact Node, Git y worktrees;
- secretos, políticas e infraestructura remota.

No se añadirán estos componentes hasta que exista una responsabilidad concreta
y una prueba que justifique su coste.

## Consecuencias

### Positivas

- el primer comando ya prueba persistencia, auditoría y reintentos;
- la semántica local no bloquea el modelo futuro para equipos;
- los consumidores pueden reanudar eventos después de una caída;
- el entorno de desarrollo no depende de Go instalado en el host;
- Git queda fuera del proceso con autoridad de datos.

### Costes

- PostgreSQL y Docker hacen más pesada la instalación personal;
- una primera migración amplia exige disciplina para evolucionar el esquema;
- el modo local todavía usa un solo rol PostgreSQL;
- no existe aún un worker que publique el outbox;
- la organización implícita no debe confundirse con aislamiento multi-tenant
  completo.

## Evidencia de aceptación

El corte se acepta cuando:

1. la imagen de pruebas compila y ejecuta tests;
2. PostgreSQL 18 instala la migración con pgvector habilitado;
3. `project.create` escribe proyecto, evento, outbox e idempotencia;
4. solicitudes concurrentes con la misma clave producen un solo proyecto;
5. la misma clave y otro payload devuelve conflicto;
6. JSON y SSE recuperan el evento mediante cursor;
7. reiniciar Pact Server conserva el proyecto y el evento.
