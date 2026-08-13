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

El token único actual es una credencial de bootstrap para una instalación
privada. Se guarda fuera del repositorio, en `~/.config/pact/config.json`, con
permisos `0600`. No es el modelo final para equipos.

Antes de invitar colaboradores externos, Pact incorporará identidad personal,
OIDC o device flow, invitaciones de un solo uso, roles por organización y
proyecto, revocación y almacenamiento de credenciales en el llavero del sistema.
Un colaborador recibirá su propia identidad; no se compartirá el token del
administrador.

## Agentes

El acceso humano y la identidad del agente son distintos. Después de conectar
la máquina, `pact agent run` genera o reutiliza una identidad privada de nodo en
`.pact/node.json` y abre una sesión de actor independiente:

```sh
pact agent run --client kimi -- kimi
```

El CLI mantiene un heartbeat mientras vive el proceso hijo y cierra la sesión
cuando termina. También inyecta `PACT_SESSION_ID`, `PACT_PROJECT_ID` y
`PACT_SERVER_URL` en el entorno del proceso, de modo que futuras integraciones
puedan atribuir comandos de dominio sin leer conversaciones.

Esta primera implementación registra presencia, no observación de Git. Declara
`observe_git=false`; por tanto, una sesión activa no autoriza al backoffice a
afirmar que el código está siendo modificado. Pact Node incorporará después la
observación de diffs y la administración de worktrees.

Clonar el repositorio no otorga por sí mismo permisos al agente. El cliente usa
la identidad con la que ese computador inició sesión en Pact Server.

## Consecuencias

- Git sigue siendo la autoridad del contenido y distribuye `pact.yaml`.
- PACT reconoce un proyecto aunque diferentes máquinas usen SSH o HTTPS.
- `.pact/` puede reconstruirse después de clonar o borrar el estado local.
- conectar es seguro por defecto y no crea proyectos por un error tipográfico;
- un cliente de IA puede publicar presencia sin entregar su conversación;
- la colaboración entre máquinas propias ya funciona con la credencial de
  bootstrap, pero invitar personas externas requiere autenticación individual;
- ver una sesión activa no equivale todavía a observar modificaciones de código.
