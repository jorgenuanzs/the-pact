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
- backoffice local para consultar proyectos, trabajo activo y eventos;
- CLI de acceso y bootstrap con `pact login`, `pact init` y `pact connect`;
- identidades personales, invitaciones de un solo uso, roles y revocación;
- identidad remota del repositorio para conectar varios checkouts sin duplicar proyectos;
- registro de nodos y sesiones vivas mediante `pact agent run`;
- entorno reproducible mediante Docker Compose.

El siguiente corte incorporará Pact Node para observar Git y administrar
worktrees; después ampliará intenciones, scopes e integraciones MCP.

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

Abre el backoffice local en:

```text
http://127.0.0.1:8080/admin/
```

La interfaz solicita un token personal o el bootstrap configurado en `.env`.
Lo conserva únicamente en `sessionStorage` dentro de la pestaña y lo utiliza
como Bearer para consultar la API; no viene incluido en los archivos del panel
ni se añade a la URL.

El panel recibe eventos duraderos en vivo y consulta el estado operativo del
proyecto. Hasta que exista Pact Node, no puede observar el sistema de archivos:
la actividad del código aparecerá como `unobserved`, que significa «Pact no lo
está observando», no «nadie está modificando código».

El backoffice utiliza las consultas autenticadas `GET /v1/projects`,
`GET /v1/projects/{project_id}/overview` y el stream SSE de eventos del
proyecto. En este corte es una superficie de observación; todavía no modifica
la coordinación ni el repositorio.

La guía de [desarrollo local](docs/development.md) contiene el flujo completo
para crear un proyecto, recuperar eventos, ejecutar pruebas y diagnosticar el
entorno.

## Conectar un repositorio

Las versiones etiquetadas publican binarios para macOS y Linux, tanto arm64
como amd64. Por ejemplo, en un Mac con Apple Silicon:

```sh
PACT_VERSION=v0.1.0
curl -fL -o pact.tar.gz \
  "https://github.com/jorgenuanzs/the-pact/releases/download/${PACT_VERSION}/pact_darwin_arm64.tar.gz"
curl -fL -o checksums.txt \
  "https://github.com/jorgenuanzs/the-pact/releases/download/${PACT_VERSION}/checksums.txt"
grep 'pact_darwin_arm64.tar.gz' checksums.txt | shasum -a 256 -c -
tar -xzf pact.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 755 pact "$HOME/.local/bin/pact"
```

Para construir el CLI desde el repositorio sin instalar Go en el host:

```sh
make cli
```

Inicia sesión una vez por computador. Para el propietario inicial, el bootstrap
se lee desde la entrada estándar para que no quede en el historial del shell:

```sh
printf '%s' "$PACT_API_TOKEN" | ./bin/pact login \
  --server https://pact.example.com \
  --token-stdin
```

Después, el propietario inicializa el proyecto desde cualquier ruta de su
repositorio Git:

```sh
cd repository
pact init
```

El comando crea el proyecto remoto, `pact.yaml`, que se versiona con Git, y
`.pact/config.json`, que es local y está ignorado. No escribe tokens ni
credenciales de PostgreSQL en el repositorio.

Un colaborador conecta otro checkout del mismo proyecto así:

```sh
git clone https://github.com/example/repository.git
cd repository
pact connect
```

PACT normaliza los remotos Git SSH y HTTPS antes de compararlos. `pact connect`
solo enlaza proyectos existentes; nunca crea silenciosamente un proyecto nuevo.
Ejecutar `pact init` en un clon que ya contiene `pact.yaml` también reutiliza el
proyecto remoto cuando el repositorio coincide.

Una vez conectado, cualquier cliente de IA puede anunciar su presencia mientras
trabaja ejecutándose a través del CLI:

```sh
pact agent run --client kimi -- kimi
pact agent run --client claude -- claude
pact agent run --client codex -- codex
```

PACT registra por separado el computador, el actor y su sesión. La sesión
permanece activa mientras vive el proceso hijo y se cierra al terminarlo; se
envían heartbeats, pero no se captura la conversación ni la entrada o salida del
agente. El comando después de `--` puede ser cualquier ejecutable local.

Este wrapper hace visible quién está conectado, pero todavía no observa los
cambios del repositorio. Hasta que exista Pact Node, el backoffice mantiene la
actividad de código en `unobserved` aunque muestre una sesión activa.

## Invitar a otra persona

El owner crea una invitación desde un checkout conectado:

```sh
pact invite create \
  --email persona@example.com \
  --role contributor
```

PACT muestra una sola vez un secreto `pact_inv_...`. Debe enviarse por un canal
privado y nunca añadirse a Git, una URL o un chat público. En el otro computador:

```sh
printf '%s' "$PACT_INVITATION" | pact join \
  --server https://pact.example.com \
  --name "Nombre de la persona" \
  --invite-stdin

git clone git@github.com:example/repository.git
cd repository
pact connect
pact agent run --client kimi -- kimi
```

`pact join` consume la invitación y guarda un token personal. `pact whoami`
muestra la identidad actual y `pact logout --revoke` retira inmediatamente ese
token del servidor antes de borrarlo del computador.

La credencial bootstrap queda reservada para recuperación y para establecer al
primer owner mediante una invitación `--role owner`; no debe compartirse con
colaboradores.

## Arquitectura del primer corte

```text
Agente o herramienta local
            │
            ▼
  Pact CLI ── sesión y heartbeat del actor
            │
  Pact Node (siguiente corte) ── observación Git + worktrees
            │ HTTPS / JSON / SSE
            ▼
        Pact Server
            │ red privada
            ▼
     PostgreSQL + pgvector
```

Pact Server no monta el repositorio del usuario ni el socket de Docker. La base
de datos permanece en una red privada y la API se publica únicamente en
loopback durante el desarrollo local.

## Documentación

- [Documento maestro](PACT_MASTER_PLAN.md)
- [ADR-0001: primer recorrido de producto](docs/adr/0001-first-product-slice.md)
- [ADR-0002: fundación de plataforma](docs/adr/0002-platform-foundation.md)
- [ADR-0003: actividad de código observada](docs/adr/0003-observed-code-activity.md)
- [ADR-0004: bootstrap local y vínculo con el servidor](docs/adr/0004-local-project-bootstrap.md)
- [ADR-0005: incorporación y conexión entre máquinas](docs/adr/0005-cli-onboarding-and-machine-connection.md)
- [ADR-0006: acceso personal, invitaciones y roles](docs/adr/0006-personal-access-and-project-roles.md)
- [Especificación del bucle central v0.1](docs/spec/core-loop-v0.1.md)
- [Contrato OpenAPI](api/openapi.yaml)
- [Desarrollo local](docs/development.md)

## Primer objetivo de producto

Demostrar que dos agentes pueden trabajar sobre el mismo repositorio, compartir
estado útil y coordinar cambios sin tener que intercambiar sus conversaciones.
