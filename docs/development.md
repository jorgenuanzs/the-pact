# Desarrollo local de Pact

Esta guía cubre el primer recorrido vertical: iniciar Pact Server con
PostgreSQL, comprobar su estado, crear y consultar un proyecto, y recuperar sus
eventos mediante JSON o Server-Sent Events (SSE).

En esta modalidad existe una organización local implícita. La API todavía no
expone operaciones para crear o seleccionar organizaciones.

## Requisitos

- Docker Desktop, o Docker Engine con el plugin Docker Compose;
- GNU Make;
- Git;
- `curl` para ejecutar los ejemplos.

No es necesario instalar Go en el host para utilizar los comandos del
repositorio: la compilación y las pruebas se ejecutan en contenedores.

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

Este perfil es exclusivamente local y confiable. Pact Server y el migrador
todavía comparten el usuario propietario de PostgreSQL; no debe exponerse la API
a una red ni utilizarse como instalación de equipo hasta separar los roles de
bootstrap, migración, runtime y workers.

Pact Node se incorporará posteriormente como un proceso del host. Será quien
observe repositorios locales y administre worktrees; Pact Server no recibirá un
montaje del repositorio del usuario ni ejecutará Git dentro del contenedor.
