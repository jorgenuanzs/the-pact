# ADR-0005 — Incorporación y conexión entre máquinas

**Estado:** aceptado
**Fecha:** 13 de agosto de 2026

## Contexto

Un repositorio puede tener varios checkouts y participantes. `pact.yaml` viaja
con Git, pero `.pact/` no: cada máquina necesita reconstruir su vínculo privado
sin crear otro proyecto remoto ni compartir la identidad de otro nodo.

Los remotos Git tampoco tienen una única representación textual. Una máquina
puede clonar mediante HTTPS y otra mediante SSH aunque ambas trabajen sobre el
mismo repositorio.

## Decisión

La incorporación se divide en tres comandos:

```text
pact login    autentica este computador contra un Pact Server
pact init     crea o recupera el proyecto y conecta el checkout propietario
pact connect  conecta un checkout a un proyecto que ya debe existir
```

### Primer participante

```sh
pact login --server https://pact.example.com --token-stdin
cd repository
pact init
```

`pact init` crea el manifiesto cuando falta, normaliza `remote.origin.url`, crea
el proyecto y su repositorio raíz en una transacción y guarda el identificador
remoto únicamente en `.pact/config.json`.

### Otro computador

```sh
git clone git@github.com:example/repository.git
cd repository
pact login --server https://pact.example.com --token-stdin
pact connect
```

`pact connect` exige que `pact.yaml` ya exista. Busca el proyecto por el remoto
Git normalizado y comprueba que una selección explícita mediante `--project`
pertenezca al mismo repositorio. Nunca crea un proyecto remoto.

`pact init` también es seguro en un clon: si el manifiesto y el repositorio ya
corresponden a un proyecto remoto, reutiliza ese proyecto.

### Identidad canónica

El identificador compartido se resuelve con el repositorio raíz registrado en
Pact Server. Las formas SSH, HTTPS y `ssh://` se normalizan a una URL HTTPS sin
credenciales ni el sufijo `.git`. La base de datos impide que un mismo remoto
activo pertenezca a dos proyectos de la organización.

El UUID del proyecto no se guarda en `pact.yaml`, porque distintas máquinas
pueden apuntar deliberadamente a instalaciones diferentes de Pact Server. Se
guarda en `.pact/config.json`, junto con la URL del servidor y sin credenciales.

## Autenticación

El token configurado en el servidor es una credencial bootstrap de recuperación
y se guarda fuera del repositorio. ADR-0006 incorporó invitaciones de un solo
uso, identidades personales, roles por organización y proyecto, vencimiento y
revocación. Un colaborador recibe su propio token y no comparte el bootstrap.

OIDC, device flow y almacenamiento nativo en el llavero siguen siendo la
evolución prevista para reemplazar la emisión local y proteger la credencial en
reposo con servicios del sistema operativo.

## Agentes

El acceso humano y la identidad del agente son distintos. Después de conectar
la máquina, `pact agent run` genera o reutiliza una identidad privada de nodo en
`.pact/node.json` y abre una sesión de actor independiente:

```sh
pact agent run --client kimi -- kimi
```

El CLI mantiene un heartbeat y observa Git mientras vive el proceso hijo;
cierra la sesión cuando termina. También inyecta `PACT_SESSION_ID`, `PACT_PROJECT_ID` y
`PACT_SERVER_URL` en el entorno del proceso, de modo que futuras integraciones
puedan atribuir comandos de dominio sin leer conversaciones.

El wrapper declara `observe_git=true` y reporta observaciones durante la sesión.
Para personas, IDE y herramientas que trabajen fuera del wrapper existe
`pact node run`, un proceso residente con la misma capacidad de observación.
La administración de worktrees sigue siendo una evolución posterior.

Clonar el repositorio no otorga por sí mismo permisos al agente. El cliente usa
la identidad con la que ese computador inició sesión en Pact Server.

## Consecuencias

- Git sigue siendo la autoridad del contenido y distribuye `pact.yaml`.
- PACT reconoce un proyecto aunque diferentes máquinas usen SSH o HTTPS.
- `.pact/` puede reconstruirse después de clonar o borrar el estado local.
- conectar es seguro por defecto y no crea proyectos por un error tipográfico;
- un cliente de IA puede publicar presencia sin entregar su conversación;
- la colaboración entre personas utiliza identidades y tokens individuales;
- solo una sesión observadora con telemetría reciente demuestra modificaciones.
