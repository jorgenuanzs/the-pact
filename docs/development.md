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

No es necesario instalar Go en el host para utilizar los comandos del
repositorio: la compilación y las pruebas se ejecutan en contenedores.

## Inicializar y conectar un proyecto

El CLI de Pact se construye como un artefacto nativo del host mediante Docker:

```sh
make cli
```

El destino detecta macOS o Linux y las arquitecturas arm64 o amd64. Primero
inicia sesión en Pact Server:

```sh
printf '%s' "$PACT_LOCAL_API_TOKEN" | ./bin/pact login \
  --server http://127.0.0.1:8080 \
  --token-stdin
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

La configuración local contiene la URL de Pact Server y el UUID remoto del
proyecto, pero no tokens ni credenciales de PostgreSQL. La credencial personal
o bootstrap se guarda fuera de todos los repositorios en
`~/.config/pact/config.json`, con permisos `0600`.

Para conectar otro checkout que recibió `pact.yaml` mediante Git:

```sh
./bin/pact connect
```

`pact connect` no crea proyectos. Compara el remoto Git normalizado con los
proyectos existentes del servidor. Consulta
[ADR-0004](adr/0004-local-project-bootstrap.md) y
[ADR-0005](adr/0005-cli-onboarding-and-machine-connection.md) para conocer la
separación de responsabilidades.

Para registrar una sesión mientras se ejecuta un agente local:

```sh
./bin/pact agent run --client kimi -- kimi
```

El CLI crea `.pact/node.json` con permisos `0600`, registra el nodo y el actor,
mantiene un heartbeat y cierra la sesión cuando termina el comando. No captura
la conversación ni la salida del proceso. Para una prueba sin un cliente de IA:

```sh
./bin/pact agent run --client test -- sleep 30
```

## Acceso personal e invitaciones

La credencial `PACT_LOCAL_API_TOKEN` es el bootstrap de recuperación. Para crear
la primera identidad owner desde un proyecto conectado:

```sh
./bin/pact invite create \
  --email owner@example.com \
  --role owner
```

Solo el bootstrap puede emitir `owner`. Un owner o maintainer puede invitar
colaboradores con `maintainer`, `contributor` o `viewer`, según sus permisos.
La invitación dura 24 horas por defecto y admite `--expires` entre `1h` y
`168h`.

El colaborador consume el secreto mediante stdin:

```sh
printf '%s' "$PACT_INVITATION" | ./bin/pact join \
  --server http://127.0.0.1:8080 \
  --name "Local collaborator" \
  --invite-stdin
```

Puede comprobar y retirar su identidad así:

```sh
./bin/pact whoami
./bin/pact logout --revoke
```

PostgreSQL conserva solamente digests de invitaciones y tokens. El secreto no
se envía en URLs y las respuestas que lo muestran declaran `Cache-Control:
no-store`. Consulta [ADR-0006](adr/0006-personal-access-and-project-roles.md).

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

## Autenticación local

Las operaciones de proyecto requieren el token Bearer configurado en `.env`.
Exporta en tu terminal el mismo valor antes de ejecutar los ejemplos:

```sh
export PACT_LOCAL_API_TOKEN='reemplaza-este-valor'
```

No copies el token en documentación, logs, commits ni historiales compartidos.

## Backoffice local

Pact Server sirve un backoffice de observación en:

```text
http://127.0.0.1:8080/admin/
```

`/admin` redirige a la ruta canónica `/admin/`. La página y sus recursos
estáticos no contienen el token ni datos del proyecto. Introduce un token
personal; el bootstrap de `.env` debe reservarse para recuperación.

El navegador conserva el token únicamente en `sessionStorage` y lo envía en el
encabezado Bearer de cada consulta. No se almacena en `localStorage`, cookies,
la URL ni Pact Server. Cerrar la pestaña o la sesión del navegador elimina ese
estado. Este mecanismo pertenece al perfil local confiable; no sustituye un
sistema de identidad para equipos.

El backoffice utiliza:

```text
GET /v1/projects
GET /v1/projects/{project_id}/overview
GET /v1/projects/{project_id}/events/stream
```

La lista permite seleccionar un proyecto. El overview reúne sus contadores,
sesiones y trabajo activos, estado de observación del código y los eventos más
recientes. El stream SSE entrega eventos duraderos recuperables por cursor.

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
manualmente. Desde un checkout conectado, su configuración debe iniciar:

```sh
pact mcp serve --client test --path /ruta/absoluta/al/checkout
```

El protocolo se comunica por `stdin`/`stdout`; no deben escribirse logs normales
en `stdout`. La identidad se carga desde la configuración privada del usuario y
los roles se siguen validando en Pact Server. Consulta
[ADR-0008](adr/0008-local-mcp-adapter.md) para conocer el contrato y su frontera
de privacidad.

## Crear un proyecto

La creación requiere una clave de idempotencia. Repetir la misma solicitud con
la misma clave devuelve el resultado original sin crear otro proyecto.

```sh
curl --fail-with-body --silent --show-error \
  --request POST \
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
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
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}"
```

## Listar proyectos

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
  http://127.0.0.1:8080/v1/projects
```

## Consultar el overview de un proyecto

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
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
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/events?limit=100"
```

Para continuar después de un cursor:

```sh
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/events?after=1&limit=100"
```

## Seguir eventos mediante SSE

`curl --no-buffer` muestra cada evento en cuanto llega:

```sh
curl --no-buffer \
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
  "http://127.0.0.1:8080/v1/projects/${PACT_PROJECT_ID}/events/stream"
```

Después de una desconexión, reanuda el stream enviando como `Last-Event-ID` el
último identificador SSE procesado:

```sh
curl --no-buffer \
  --header "Authorization: Bearer ${PACT_LOCAL_API_TOKEN}" \
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
