# Desarrollo local de Pact

Esta guía cubre el primer recorrido vertical: iniciar Pact Server con
PostgreSQL, comprobar su estado, utilizar el backoffice local, crear y consultar
un proyecto, y recuperar sus eventos mediante JSON o Server-Sent Events (SSE).

En esta modalidad existe una organización local implícita. La API todavía no
expone operaciones para crear o seleccionar organizaciones.

## Requisitos

- Docker Desktop, o Docker Engine con el plugin Docker Compose;
- GNU Make;
- Git;
- `curl` para ejecutar los ejemplos.

Estos requisitos corresponden al desarrollo del servidor. Un colaborador que
solo utiliza el CLI nativo en Windows necesita Git for Windows y `pact.exe`; su
PostgreSQL continúa viviendo junto al Pact Server central.

No es necesario instalar Go en el host para utilizar los comandos del
repositorio: la compilación y las pruebas se ejecutan en contenedores.

## Inicializar y conectar un proyecto

El CLI de Pact se construye como un artefacto nativo del host mediante Docker:

```sh
make cli
```

El destino detecta macOS o Linux y las arquitecturas arm64 o amd64. Primero
inicia sesión en Pact Server. El CLI abre el navegador para confirmar este
dispositivo con tu cuenta:

```sh
./bin/pact login --server http://127.0.0.1:8080
```

El mismo computador puede mantener varios perfiles autorizados. El perfil
activo es solamente la preferencia para comandos que no se ejecutan dentro de
un checkout vinculado:

```sh
./bin/pact login --server https://pact.example.com --name "Equipo remoto"
./bin/pact servers list
./bin/pact servers use http://127.0.0.1:8080
```

Después ejecuta:

```sh
./bin/pact init
```

`pact init` busca la raíz Git y crea dos superficies diferentes:

```text
pact.yaml          manifiesto compartido, debe versionarse con Git
.pact/config.json  vínculo de esta máquina, está ignorado por Git
```

La configuración del checkout contiene la URL de Pact Server, los UUID de
workspace, repositorio y proyecto, la fecha de configuración y un fingerprint
SHA-256 del remoto Git normalizado. No contiene la URL Git original,
contraseñas ni credenciales de PostgreSQL. El registro global contiene solo
metadatos de los perfiles. Las credenciales revocables de dispositivo se
guardan en macOS Keychain, Windows Credential Manager o el keyring nativo del
usuario. Un comando ejecutado dentro de un checkout resuelve siempre el perfil
indicado por `.pact/config.json`; nunca utiliza en silencio la credencial de
otro servidor activo.

Para conectar otro checkout que recibió `pact.yaml` mediante Git:

```sh
./bin/pact connect
```

`pact connect` no crea proyectos. Compara el remoto Git normalizado con los
repositorios existentes y valida que el repositorio pertenezca al workspace
seleccionado. Usa `--workspace UUID` o `--repository UUID` para desambiguar.
Cambiar un vínculo existente requiere `--rebind`; la escritura es atómica y la
identidad del nodo rota cuando cambia el servidor. Consulta
[ADR-0004](adr/0004-local-project-bootstrap.md) y
[ADR-0019](adr/0019-desktop-multi-server-and-folder-bindings.md) para conocer
la separación de responsabilidades.

Para registrar una sesión mientras se ejecuta un agente local:

```sh
./bin/pact agent run --client kimi -- kimi
```

El CLI crea `.pact/node.json` como estado privado local (`0600` en Unix),
registra el nodo y el actor,
mantiene un heartbeat y cierra la sesión cuando termina el comando. No captura
la conversación ni la salida del proceso. Para una prueba sin un cliente de IA:

```sh
./bin/pact agent run --client test -- sleep 30
```

## Cuentas, dispositivos e invitaciones

`PACT_SETUP_TOKEN` sirve solamente para crear la primera cuenta propietaria en
el backoffice y debe retirarse después del entorno del servidor. Una cuenta con
permisos puede invitar a un colaborador desde un proyecto conectado:

```sh
./bin/pact invite create \
  --email collaborator@example.com \
  --role contributor
```

Un owner de la organización puede emitir `owner`. Un owner o maintainer del
proyecto puede invitar otros roles según sus permisos.
La invitación dura 24 horas por defecto y admite `--expires` entre `1h` y
`168h`.

El comando devuelve una URL de registro privada. Si el colaborador recibe solo
el secreto, puede abrir esa misma pantalla mediante:

```sh
printf '%s' "$PACT_INVITATION" | ./bin/pact join \
  --server http://127.0.0.1:8080 \
  --invite-stdin
```

Puede comprobar y retirar su identidad así:

```sh
./bin/pact login --server http://127.0.0.1:8080
./bin/pact servers list
./bin/pact whoami
./bin/pact logout --server http://127.0.0.1:8080
```

PostgreSQL conserva solamente digests de invitaciones, sesiones y credenciales
de dispositivo. El secreto de invitación viaja en el fragmento `#invite` de la
URL de registro, que el navegador no envía al servidor; la aplicación lo
intercambia en el cuerpo de una solicitud con `Cache-Control: no-store`.
Consulta [ADR-0017](adr/0017-local-accounts-web-sessions-and-device-authorization.md).

## Preparación

Desde la raíz del repositorio:

```sh
make init
```

Revisa `.env` antes de iniciar los servicios. El archivo contiene valores
locales y no debe añadirse a Git.

Inicia el entorno:

```sh
make dev
```

`make dev` construye e inicia Pact Server y PostgreSQL. La API se publica
únicamente en la interfaz de loopback:

```text
http://127.0.0.1:8080
```

Comprueba el estado de los contenedores y sigue los logs con:

```sh
make ps
make logs
```

## Higiene de Docker

Pact utiliza un builder BuildKit exclusivo llamado `the-pact-builder`. Esto
separa su caché de compilación de la de otros proyectos. Las pruebas eliminan
sus imágenes temporales al terminar y `make verify` poda automáticamente la
caché de Pact con más de siete días y limita la caché reciente a 1 GB.

El contexto de compilación excluye `infra/`, estados, planes y variables de
Terraform. Además de evitar cientos de megabytes innecesarios en cada build,
esto impide que configuración local de infraestructura entre en las imágenes
temporales de desarrollo.

Audita solamente los recursos Docker de Pact con:

```sh
make docker-audit
```

Elimina contenedores detenidos, imágenes y caché antiguas de Pact con:

```sh
make docker-clean-stale
```

Para retirar todo el entorno local de Pact excepto los datos de PostgreSQL:

```sh
make docker-clean
```

Ninguno de estos comandos elimina volúmenes. El volumen
`the-pact_pact_postgres_data` se conserva deliberadamente. No uses
`docker volume prune` ni `docker system prune --volumes` sin confirmar antes
que existen respaldos de las bases de datos afectadas.

## Salud y versión

La comprobación de vida solo confirma que el proceso HTTP responde:

```sh
curl --fail-with-body --silent --show-error \
  http://127.0.0.1:8080/livez
```

La comprobación de disponibilidad también valida que Pact terminó de arrancar,
que el esquema es compatible y que PostgreSQL está accesible:

```sh
curl --fail-with-body --silent --show-error \
  http://127.0.0.1:8080/readyz
```

Consulta la versión:

```sh
curl --fail-with-body --silent --show-error \
  http://127.0.0.1:8080/version
```

Las respuestas exitosas utilizan un sobre `data`, por ejemplo:

```json
{"data":{"status":"ready"}}
```

## Autenticación para ejemplos de API

Las operaciones de proyecto aceptan la credencial del dispositivo emitida por
`pact login`. PACT la guarda en el almacén seguro del sistema operativo y no la
expone desde `config.json` ni mediante un comando de exportación.

Los ejemplos de bajo nivel con `curl` que aparecen más adelante esperan una
credencial efímera proporcionada explícitamente por un fixture de integración:

```sh
export PACT_DEVICE_CREDENTIAL="pact_device_<integration-fixture-only>"
```

No utilices una credencial real de Desktop o CLI para estos ejemplos ni la
copies en documentación, logs, commits o historiales compartidos.

## Backoffice local

Pact Server sirve un backoffice de observación en:

```text
http://127.0.0.1:8080/admin/
```

`/admin` redirige a la ruta canónica `/admin/`. La página y sus recursos
estáticos no contienen credenciales ni datos del proyecto. El usuario inicia
sesión con usuario o correo y contraseña. Pact entrega una cookie HttpOnly,
SameSite=Strict y, en HTTPS, Secure; las mutaciones requieren además el secreto
CSRF ligado a la sesión.

El backoffice utiliza:

```text
GET /v1/projects
GET /v1/projects/{project_id}/overview
GET /v1/projects/{project_id}/events/stream
GET /v1/workspaces/{workspace_id}/rooms
GET /v1/workspaces/{workspace_id}/rooms/{room_id}/messages
POST /v1/workspaces/{workspace_id}/rooms/{room_id}/messages
GET /v1/me/room-mentions?workspace_id={workspace_id}
GET /v1/admin/users
PATCH /v1/admin/users/{principal_id}
PUT /v1/admin/users/{principal_id}/projects/{project_id}
POST /v1/admin/invitations
```

La lista permite seleccionar un proyecto. El overview reúne sus contadores,
sesiones y trabajo activos, estado de observación del código y los eventos más
recientes. El stream SSE entrega eventos duraderos recuperables por cursor.

Al seleccionar el encabezado de un Workspace, Pact Control abre sus rooms de
contexto. Cada Workspace recibe `#general`; un maintainer puede crear otras
rooms manualmente. El compositor resuelve `@` contra personas y agentes
elegibles, y crea una notificación durable solo para los actores elegidos. El
cliente consulta únicamente los últimos cincuenta mensajes de la room abierta
y actualiza esa ventana cada cinco segundos. Las rooms no crean intents,
scopes, ramas ni worktrees. Consulta
[ADR-0016](adr/0016-workspace-context-rooms.md).

Los propietarios y administradores ven además **Usuarios y permisos** en la
navegación organizacional. Esa sección administra cuentas, invitaciones,
sesiones y acceso por proyecto. Desactivar sustituye a eliminar: revoca los
accesos inmediatamente y conserva la atribución histórica. Las protecciones
del último propietario y de la cuenta actual se aplican también en PostgreSQL,
no dependen de la interfaz. Consulta
[ADR-0018](adr/0018-organization-user-administration.md).

La actividad del código utiliza estos estados:

- `unobserved`: Pact no cuenta con un observador vigente ni evidencia reciente;
  el estado real del repositorio es desconocido;
- `idle`: existe un observador vigente y no informó cambios recientes;
- `editing`: Pact recibió evidencia de una modificación durante los últimos
  segundos;
- `recent`: existe evidencia de un cambio reciente, pero no de que continúe.

Una sesión o workspace activo no demuestra edición. Además, `editing` significa
que Pact detectó una modificación reciente, no que pueda observar pulsaciones
de teclado. Consulta
[ADR-0003](adr/0003-observed-code-activity.md) para conocer las reglas y
ventanas exactas.

El panel combina SSE con una consulta periódica del overview. Los eventos son
duraderos, pero los heartbeats actualizan `last_seen_at` sin crear un evento en
cada intervalo; el polling permite reflejar conexiones que quedan obsoletas.

`pact agent run` observa Git mientras vive el agente. Para cubrir cambios
realizados fuera de ese wrapper, ejecuta desde el checkout conectado:

```sh
pact node run
```

Pact Node anuncia la capacidad de observar diffs, mantiene heartbeat y reporta
solo cuando cambia la huella local. `pact node run --once` resulta útil para una
comprobación puntual. El panel no examina por sí mismo el sistema de archivos
del host: si ningún observador está vigente, el indicador correcto sigue siendo
`unobserved`.

La telemetría no contiene nombres ni contenido de archivos. Incluye dirty/clean,
revisión, rama, cantidad de rutas y una huella SHA-256 calculada a partir del
estado Git y metadatos locales. Un cambio de huella dirty emite
`pact.workspace.diff_updated.v1`; un cambio de HEAD sin diff emite
`pact.git.external_change_detected.v1`.

Un cliente MCP puede consumir la misma vista operativa sin ejecutar el CLI
manualmente. Codex y Claude Code se habilitan de forma local e idempotente con:

```sh
pact enable codex
pact enable claude
```

Codex utiliza `.codex/config.toml`; Claude Code utiliza `.mcp.json`. Cuando Pact
crea esos archivos los excluye mediante `.git/info/exclude`, ya que contienen
rutas absolutas propias de cada checkout. Claude Code solicita aprobación antes
de iniciar por primera vez un servidor MCP configurado en el proyecto.

PACT Desktop ofrece el mismo flujo desde **Este computador → Agentes y
clientes**. El usuario selecciona explícitamente un checkout con el diálogo
nativo y Desktop instala la configuración usando el runtime `pact-local`
incluido en la aplicación. El runtime se extrae en el directorio de
configuración del usuario, identificado por su digest, y puede ser iniciado por
el cliente MCP aunque la interfaz Desktop esté cerrada. La autorización de
dispositivo permanece en la configuración privada del usuario; nunca se copia
al checkout ni se expone al frontend React.

Para probar el transporte directamente o configurar otro cliente, el proceso
que debe iniciarse desde un checkout conectado es:

```sh
pact mcp serve --client test --path /ruta/absoluta/al/checkout
```

El protocolo se comunica por `stdin`/`stdout`; no deben escribirse logs normales
en `stdout`. La identidad se carga desde la configuración privada del usuario y
los roles se siguen validando en Pact Server. Consulta
[ADR-0008](adr/0008-local-mcp-adapter.md) para conocer el contrato y su frontera
de privacidad.

## Sincronizar los repositorios del proyecto

Desde un checkout conectado, PACT puede contrastar su revisión compartida con
la rama por defecto real de GitHub:

```sh
pact repository status
pact repository sync
pact repository list
pact repository status --repository REPOSITORY_UUID
pact repository sync --repository REPOSITORY_UUID
```

Las operaciones de sincronización requieren rol `maintainer`. El repositorio
principal mantiene la proyección histórica `project.canonical_revision`; los
repositorios adicionales conservan una revisión verificada independiente.

En producción, configura la GitHub App descrita en el README y selecciona los
repositorios desde Pact Control. `PACT_GITHUB_TOKEN` queda como alternativa de
desarrollo o GHES y nunca debe aparecer en `pact.yaml`, `.pact/` ni en la
configuración MCP del proyecto. El polling automático es opcional y se activa
con un intervalo mínimo de un minuto:

```sh
PACT_GITHUB_SYNC_INTERVAL=5m
```

Consulta [ADR-0014](adr/0014-github-canonical-repository-sync.md) y
[ADR-0015](adr/0015-github-app-and-multi-repository-projects.md) para conocer
el modelo persistido, la verificación de instalaciones y el conjunto de
revisiones por proyecto.

## Crear un proyecto

La creación requiere una clave de idempotencia. Repetir la misma solicitud con
la misma clave devuelve el resultado original sin crear otro proyecto.

```sh
curl --fail-with-body --silent --show-error \
  --request POST \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: local-create-the-pact-001' \
  --data '{
    "name": "The Pact",
    "slug": "the-pact",
    "canonical_revision": "3269b3f"
  }' \
  http://127.0.0.1:8080/v1/projects
```

`canonical_revision` es opcional. Si todavía no existe una revisión canónica,
puede omitirse:

```json
{
  "name": "The Pact",
  "slug": "the-pact"
}
```

La respuesta contiene el proyecto dentro de `data`. Copia su `id` para las
consultas siguientes:

```sh
export PACT_PROJECT_ID='identificador-devuelto-por-la-api'
```

## Consultar un proyecto

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}"
```

## Listar proyectos

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  http://127.0.0.1:8080/v1/projects
```

## Consultar el overview de un proyecto

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/overview"
```

La respuesta incluye `project`, `code_activity`, `counts`, `active_work`,
`recent_events` y `generated_at`. Los arrays vacíos se devuelven como `[]`, no
como `null`.

## Consultar eventos

Los eventos se ordenan mediante un cursor por proyecto. Aunque en esta versión
el cursor es la representación decimal de un `bigint`, debe tratarse como una
cadena opaca: se almacena y se devuelve sin transformarlo.

Para obtener los primeros cien eventos:

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/events?limit=100"
```

Para continuar después de un cursor:

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/events?after=1&limit=100"
```

## Seguir eventos mediante SSE

`curl --no-buffer` muestra cada evento en cuanto llega:

```sh
curl --no-buffer \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/events/stream"
```

Después de una desconexión, reanuda el stream enviando como `Last-Event-ID` el
último identificador SSE procesado:

```sh
curl --no-buffer \
  --header "Authorization: Bearer ${PACT_DEVICE_CREDENTIAL}" \
  --header 'Last-Event-ID: 1' \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/events/stream"
```

El servidor reproduce primero los eventos duraderos posteriores a ese cursor y
después continúa con eventos nuevos. Los comentarios SSE son heartbeats y
pueden ignorarse.

## Migraciones

Aplica las migraciones pendientes con:

```sh
make migrate
```

El servicio `migrate` es una tarea separada y finaliza después de aplicar el
esquema. La separación de roles de PostgreSQL entre migraciones y ejecución se
incorporará junto con el modelo de permisos para equipos.

## Pruebas y compilación

El código fuente de Pact Control vive en `web/` y se construye con React,
TypeScript y Vite. El build reproducible y CI usan Node 24.16.0; utiliza esa
versión o una Node 24 posterior para los comandos locales. Para trabajar con
recarga en caliente, primero inicia Pact Server y después Vite en otra terminal:

```sh
make dev
make ui-install
make ui-dev
```

Abre `http://127.0.0.1:5173/admin/`. Vite reenvía `/v1`, `/livez`, `/readyz` y
`/version` al servidor local, de modo que cookies, CSRF y SSE conservan el mismo
modelo de origen que en producción. `make ui-build` genera
`internal/transport/httpapi/adminui/dist/` para las comprobaciones nativas de
Go; Docker siempre lo genera desde cero y Node no forma parte de la imagen
final.

Para iterar exclusivamente en el backoffice ejecuta el perfil rápido:

```sh
make test-ui
```

Este perfil ejecuta el análisis de tipos de TypeScript, las pruebas de
componentes con Vitest, el build de producción de Vite, el formato y análisis
de Go del paquete `adminui`, y sus pruebas específicas. No inicia PostgreSQL ni
ejecuta las pruebas del resto del servidor. Si un cambio afecta la API,
migraciones, infraestructura o cualquier otro paquete, usa la batería completa.

Ejecuta las pruebas:

```sh
make test
```

Ejecuta también el detector de carreras de Go:

```sh
make test-race
```

Comprueba idempotencia concurrente, eventos y outbox contra PostgreSQL real:

```sh
make test-integration
```

Esta prueba utiliza un PostgreSQL efímero separado y lo elimina al finalizar;
no escribe en la base persistente del entorno de desarrollo.

Construye los binarios:

```sh
make build
```

La verificación completa agrupa formato, análisis estático, pruebas y
comprobaciones definidas por el repositorio:

```sh
make verify
```

El despliegue de OCI aplica la misma distinción automáticamente. Compara el
contenido del release nuevo con el release activo y solo utiliza el perfil
rápido cuando todos los archivos modificados viven bajo `web/` o
`internal/transport/httpapi/adminui/`; ante cualquier duda utiliza el perfil
completo.

## Diagnóstico

Ejecuta las comprobaciones de configuración, Docker, conectividad, migraciones
y salud:

```sh
make doctor
```

Para investigar un fallo:

```sh
make ps
make logs
```

## Detener el entorno

```sh
make down
```

Este comando detiene los servicios sin convertir el borrado de los datos de
PostgreSQL en una operación implícita.

## Límites del entorno actual

PostgreSQL permanece en la red privada de Docker Compose y no se publica en una
interfaz del host. Los clientes hablan con Pact Server, no directamente con la
base de datos.

Pact Server y el migrador todavía comparten el usuario propietario de
PostgreSQL. Deben separarse los roles de migración, runtime y workers antes de
considerar el endurecimiento de producción completo.

Pact Node todavía se ejecuta en primer plano. MCP se ofrece localmente por
`stdio`; todavía no existe un endpoint MCP remoto con OAuth. Los agentes MCP
pueden declarar intenciones, reservar scopes y crear worktrees aislados en
`.pact/worktrees/<intent-id>`. PACT no es un sandbox del sistema operativo: una
herramienta con permisos sobre el checkout puede ignorar el protocolo, aunque
Pact Node seguirá observando y registrando esos cambios externos.

MCP también permite ofrecer y aceptar Handoffs estructurados y compilar Context
Packs de corta duración. Aceptar un Handoff confirma su recepción, pero no
transfiere el worktree local, las reservas de scope ni la responsabilidad del
emisor. El receptor debe comenzar su propia intención coordinada después de que
el trabajo anterior libere sus scopes.

El backoffice muestra actividad observada y eventos en tiempo real, pero es una
superficie de lectura. Pact Server solo acepta comandos de dominio autenticados;
no existe un endpoint genérico para inventar eventos desde un cliente.

## Relación entre local, servidor y PostgreSQL

En el perfil personal, Pact Server y PostgreSQL se ejecutan en la misma máquina
mediante Docker. En un equipo, Pact Server se instala en infraestructura común
y cada persona configura su `.pact/config.json` para conectarse a ese endpoint.

En ambos casos el recorrido es el mismo:

```text
agente → adaptador MCP / Pact Node local → Pact Server → PostgreSQL privado
```

Los Pact Nodes nunca deben conectarse directamente a PostgreSQL. El servidor es
la autoridad que autentica, valida permisos e idempotencia, coordina sesiones y
registra eventos. PostgreSQL permanece en una red privada accesible solamente
por Pact Server, migradores y workers autorizados.

La instalación remota para equipos utiliza HTTPS, identidad personal y roles de
proyecto. La identidad privada del nodo sigue siendo local al checkout. Todavía
faltan rotación criptográfica de identidad de nodo, autorización
multi-organización completa y roles de base de datos separados. El bootstrap no
debe compartirse ni utilizarse como credencial diaria.
