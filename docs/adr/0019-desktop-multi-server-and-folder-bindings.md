# ADR-0019 — PACT Desktop multi-servidor y vínculos locales por carpeta

**Estado:** aceptado
**Fecha:** 21 de agosto de 2026  
**Propone reemplazar parcialmente:** ADR-0005, en lo relativo a una única
conexión de usuario por computador  
**Complementa:** ADR-0008, ADR-0011, ADR-0015 y ADR-0017

## Contexto

PACT distingue el estado compartido del estado local:

- PACT Server conserva workspaces, repositorios, contexto, coordinación,
  permisos y auditoría;
- PACT Desktop administra este computador, sus carpetas, clientes de IA y el
  runtime MCP;
- `.pact/config.json` vincula un checkout local con una entidad remota sin
  guardar credenciales;
- `pact.yaml` viaja con Git, mientras que `.pact/` no se comparte.

La primera versión de Desktop reutilizó el modelo inicial del CLI y guarda una
sola pareja global `server_url + device_credential`. Una carpeta solo puede
activar Codex o Claude después de ejecutar `pact init` o `pact connect`, y
Desktop rechaza cualquier binding cuyo servidor no coincida con esa conexión
global.

Ese modelo produce tres problemas:

1. El modal **Conectar cliente** mezcla la incorporación de una carpeta con la
   instalación de MCP, pero solo implementa la segunda operación.
2. La interfaz presenta **Este computador** dentro del servidor activo, aunque
   el estado local existe por encima de cualquier servidor.
3. Una persona no puede utilizar simultáneamente carpetas asociadas a PACT
   Servers diferentes, incluso si dispone de autorización válida en todos.

El caso objetivo es legítimo y necesario:

```text
PACT Desktop
├── /dev/footfall → Nuanzs Cloud / Footfall / footfall-api
├── /dev/client-a → Client A Cloud / Platform / backend
└── /dev/demo     → Local PACT / Demo / demo
```

Cada agente debe descubrir el servidor correcto a partir de la carpeta donde
se inicia. Cambiar la vista activa de Desktop no puede redirigir silenciosamente
una sesión, mezclar contexto entre servidores ni sustituir sus credenciales.

## Vocabulario

- **Servidor:** una instalación independiente de PACT Server.
- **Perfil de servidor:** conexión local autorizada desde este usuario del
  sistema operativo hacia un servidor concreto.
- **Servidor activo:** perfil cuya administración remota está visible en
  Desktop. Es una preferencia de navegación, no una autoridad global.
- **Servidor local administrado:** PACT Server instalado por Desktop mediante
  Docker. Es un servidor normal con un perfil de tipo `managed_local`.
- **Carpeta o checkout:** raíz Git local elegida explícitamente por la persona.
- **Binding:** relación privada y exclusiva entre un checkout y
  servidor + workspace + repositorio.
- **Cliente de IA:** Codex, Claude Code u otro consumidor MCP instalado o
  configurable en este computador.
- **Runtime local:** ejecutable de PACT que atiende MCP, observa Git y abre la
  sesión remota correspondiente al binding.

La capa `Project` permanece en el modelo interno por compatibilidad con los
contratos actuales. La experiencia de producto vincula una carpeta con un
Workspace y un Repository; `project_id` se deriva del repositorio mientras la
transición interna siga siendo necesaria.

## Fuerzas de diseño

- un computador puede conectarse a cero, uno o varios servidores;
- una interrupción de un servidor no debe inutilizar las demás carpetas;
- las credenciales de servidores distintos nunca se comparten ni se copian a
  configuraciones MCP;
- una carpeta debe tener un destino inequívoco;
- un workspace puede contener varios repositorios y cada checkout representa
  uno de ellos;
- el flujo gráfico debe incorporar carpetas sin exigir comandos previos;
- CLI, Desktop y runtime deben resolver el mismo binding;
- el servidor no debe recibir ni almacenar rutas absolutas del computador;
- la migración desde la configuración v2 actual no debe exigir volver a
  conectar todas las carpetas.

## Decisión

### 1. Desktop es el plano de control de este computador

PACT Desktop abrirá su superficie local aunque no exista una conexión remota.
Desde ella se administran:

- perfiles de servidores;
- carpetas locales y sus bindings;
- integraciones de clientes de IA;
- runtime local;
- instalación, ciclo de vida y actualización de un PACT Server local.

La administración de workspaces, personas, contexto y repositorios sigue
perteneciendo al servidor seleccionado. Desktop puede mostrar esa aplicación,
pero no fusiona sus datos con los de otros servidores.

### 2. Un computador conserva varios perfiles de servidor

El registro local incorpora `ServerProfile`:

```text
ServerProfile
  id                  UUID local estable
  label               nombre elegido o sugerido
  server_url          URL normalizada
  kind                remote | managed_local
  principal_id        identidad remota observada
  principal_label     nombre o correo para presentación
  credential_ref      referencia opaca al almacén seguro
  created_at
  last_used_at
```

Invariantes:

- existe como máximo un perfil por URL normalizada para cada usuario del
  sistema operativo;
- volver a autorizar la misma URL actualiza ese perfil, no crea un duplicado;
- `active_profile_id` solo indica qué servidor se está navegando;
- eliminar un perfil no elimina datos del servidor;
- un perfil con carpetas vinculadas no puede eliminarse sin resolver u
  orphanizar explícitamente esos bindings;
- `remote` exige HTTPS y `managed_local` exige una dirección loopback HTTP o
  HTTPS válida.

### 3. Las credenciales se almacenan por perfil en el sistema operativo

`device_credential` deja de vivir dentro del JSON de configuración general.
Desktop y CLI usan una interfaz común `CredentialStore` con estas
implementaciones:

- macOS Keychain;
- Windows Credential Manager;
- almacén en memoria para pruebas;
- fallback de archivo protegido solo para plataformas CLI sin almacén nativo,
  explícito y documentado.

El registro de perfiles conserva únicamente `credential_ref`. Ninguna
credencial aparece en `.pact/`, configuración MCP, eventos, logs, exportaciones
o respuestas entregadas al frontend.

La primera implementación comparte un servicio del almacén nativo entre
Desktop, CLI y `pact-local`. En macOS usa Keychain; en Windows, Credential
Manager; en sistemas compatibles de CLI, el keyring de la sesión. Estos
almacenes protegen el secreto en reposo y frente a otros usuarios del equipo,
pero no son una barrera contra código hostil que ya se ejecuta como el mismo
usuario. El fallback de archivo requiere seleccionar explícitamente
`PACT_CREDENTIAL_STORE=file`; nunca se activa de forma silenciosa.

### 4. Cada checkout tiene un único binding

Un checkout se vincula exactamente con:

```text
FolderBinding
  root                ruta local canónica, solo en Desktop
  server_url          identidad durable del servidor
  workspace_id
  repository_id
  project_id          compatibilidad transitoria
  git_remote          remoto normalizado o fingerprint
  enabled_clients     codex | claude | ...
  configured_at
```

La parte portable entre CLI y runtime vive en `.pact/config.json` con esquema
v2:

```json
{
  "schema_version": 2,
  "server_url": "https://pact.example.com",
  "workspace_id": "…",
  "repository_id": "…",
  "project_id": "…"
}
```

`project_id` permanece mientras existan endpoints que lo requieran. El
registro de Desktop puede guardar `server_profile_id` como índice local, pero
el archivo del checkout conserva `server_url`, porque un UUID local no puede
reconstruirse en otro computador.

Una misma carpeta no puede apuntar simultáneamente a varios servidores. Si se
necesita utilizar el mismo repositorio con dos PACT Servers, se crean checkouts
o worktrees separados, cada uno con su binding privado. Así una orden MCP nunca
depende de una selección implícita.

### 5. Vincular carpeta y habilitar cliente son operaciones distintas

Desktop deja de asumir que la carpeta ya está conectada. El flujo **Añadir
carpeta a PACT** realiza:

1. seleccionar e inspeccionar el checkout Git;
2. reconocer un binding existente o elegir un perfil de servidor;
3. resolver workspace y repositorio;
4. confirmar o crear el binding local;
5. habilitar uno o varios clientes de IA de forma idempotente.

La carpeta determina el destino del agente. El cliente no se conecta
globalmente a un workspace: Codex o Claude quedan habilitados por carpeta y
pueden estar habilitados en muchas carpetas de servidores distintos.

### 6. El servidor resuelve repositorios sin conocer rutas locales

PACT Server ofrecerá una operación autorizada de resolución a partir de una
identidad Git normalizada. La petición no incluye la ruta absoluta local y el
servidor vuelve a normalizar cualquier remoto recibido antes de buscar.

La resolución puede producir:

- una coincidencia exacta accesible: se preselecciona;
- varias candidatas: la persona elige;
- ninguna coincidencia: se permite elegir manualmente o adjuntar/crear un
  repositorio si el rol lo autoriza;
- coincidencia sin acceso: se informa sin revelar metadatos privados.

La respuesta identifica workspace, project transitorio, repository y el nivel
de permiso. Resolver no crea una sesión de agente ni registra la ruta local.

### 7. El runtime resuelve credenciales desde el binding

La configuración MCP continúa siendo específica de la carpeta y puede seguir
ejecutando:

```text
pact mcp serve --client <tipo> --path <checkout>
```

Al iniciar, el runtime:

1. carga `.pact/config.json`;
2. obtiene `server_url` y el repositorio remoto;
3. encuentra el perfil por URL normalizada;
4. solicita su credencial al almacén seguro;
5. verifica que el principal conserva acceso al workspace y repositorio;
6. abre la sesión, observación Git y heartbeat en ese servidor.

Dos clientes pueden ejecutar simultáneamente runtimes contra servidores
diferentes. `active_profile_id` no participa en esta resolución.

### 8. El servidor local administrado es un perfil normal

Instalar PACT Server local crea o actualiza un perfil `managed_local`. Iniciar,
detener, actualizar, respaldar o eliminar sus contenedores pertenece a la
sección **Servidor local**, no al binding de carpetas.

Detener el servidor local no afecta perfiles remotos. Eliminarlo exige una
confirmación separada y nunca elimina carpetas Git. Sus workspaces dejan de
estar disponibles hasta restaurar o iniciar ese servidor.

### 9. La interfaz separa contexto local y contexto remoto

La navegación local tendrá, como mínimo:

```text
Este computador
├── Resumen local
├── Conexiones PACT
├── Carpetas
├── Clientes de IA
└── Servidor local
```

La navegación remota siempre mostrará el perfil activo, su nombre y URL. Los
workspaces, menciones, cuenta, permisos y administración pertenecen solamente
a ese perfil.

Entrar a **Este computador** desde un servidor puede preseleccionarlo para una
nueva carpeta, pero no limita la lista de perfiles disponibles.

### 10. El modal es una máquina de estados explícita

El flujo presenta estos estados, sin errores técnicos sin traducir:

- carpeta no Git;
- carpeta sin binding;
- binding válido y perfil autorizado;
- binding válido pero perfil ausente o autorización vencida;
- carpeta vinculada a otro servidor;
- remoto con coincidencia única, múltiple o inexistente;
- permisos insuficientes;
- binding completado con una integración de cliente fallida.

La acción final usa un texto que describe ambas operaciones, por ejemplo
**Vincular carpeta y conectar Codex**.

Persistir el binding y configurar clientes es reintentable. Si MCP falla
después de crear el binding, Desktop conserva el binding válido, marca el
cliente como pendiente y ofrece reintentar. No presenta toda la operación como
fallida ni deja archivos parcialmente sobrescritos.

### 11. Rebinding es explícito y seguro

Cambiar el servidor, workspace o repositorio de una carpeta requiere una
acción **Cambiar vínculo** con resumen de origen y destino. Antes de escribir:

- se verifica la autorización del nuevo perfil;
- se detienen o invalidan sesiones locales activas para esa carpeta;
- se reemplaza `.pact/config.json` atómicamente;
- se rota la identidad privada del nodo si cambia el servidor;
- se revalida la configuración de clientes;
- se conserva un registro local sin secretos del cambio.

Desktop nunca cambia un binding únicamente porque la persona cambió el
servidor visible.

## Compatibilidad y migración

La migración se ejecuta una sola vez y es reintentable:

1. la configuración global v2 se convierte en el primer `ServerProfile`;
2. la credencial se escribe y verifica en el almacén seguro;
3. el secreto antiguo se elimina del JSON solo después de comprobar la nueva
   referencia;
4. `active_profile_id` apunta al perfil migrado;
5. cada registro local v1 se asocia al perfil por `server_url`;
6. cada `.pact/config.json` v1 conserva `project_id` y se amplía con workspace y
   repositorio cuando el servidor esté disponible;
7. si el servidor no responde, el binding legacy sigue siendo legible y la
   migración se reanuda después.

Los comandos `pact init`, `pact connect`, `pact login` y `pact mcp serve`
continúan disponibles. `pact login --server URL` añade o reautoriza un perfil
en vez de reemplazar todos los demás. `pact servers list`, `pact servers use` y
`pact servers remove` permiten administrarlos explícitamente, mientras que
`pact status` muestra la resolución efectiva de una carpeta.

## Seguridad y privacidad

- la ruta local nunca se envía a PACT Server;
- los remotos Git se limpian de credenciales antes de persistir, comparar o
  transmitir;
- el frontend recibe metadatos de perfil, nunca la credencial ni su referencia
  utilizable;
- el runtime solicita una credencial solo para la URL de su binding;
- la autorización se vuelve a comprobar en el servidor al iniciar cada sesión;
- quitar un perfil elimina su credencial local y revoca el dispositivo remoto
  cuando sea posible;
- un fallo de revocación remota se informa y no impide retirar el secreto
  local;
- logs y diagnósticos usan `profile_id`, host redactado y códigos de error, no
  tokens ni rutas completas por defecto;
- pruebas de regresión garantizan que un binding del servidor A no puede usar
  la credencial del servidor B.

## Operación sin conexión

Desktop puede listar perfiles, carpetas y clientes con su último estado aunque
un servidor esté fuera de línea. No permite crear o cambiar un binding remoto
sin verificar destino y permisos. Los clientes ya configurados pueden iniciar
el runtime, pero la sesión mostrará claramente que no alcanzó al servidor y no
simulará presencia ni coordinación compartida.

## Alternativas descartadas

### Mantener un único servidor activo y exigir cambiar de sesión

Es simple, pero impide agentes simultáneos, convierte la navegación en
autoridad implícita y facilita atribuir trabajo al servidor incorrecto.

### Guardar una credencial dentro de cada carpeta

Facilita la resolución, pero multiplica secretos, los acerca al repositorio y
aumenta el riesgo de commits, backups y exposición a agentes.

### Permitir varios bindings en el mismo checkout

Obliga a elegir un servidor en cada operación, vuelve ambiguo MCP y facilita
mezclar contexto. Los checkouts separados hacen explícita esa frontera.

### Conectar Codex o Claude globalmente al computador

La instalación del cliente sí es global, pero su configuración PACT debe ser
por carpeta. Un agente abierto en un checkout necesita un destino determinista.

### Fusionar los workspaces de todos los servidores en una vista única

Oculta fronteras de identidad, permisos y disponibilidad. Una futura búsqueda
federada podría agregarse con etiquetas de procedencia, pero no forma parte de
esta decisión.

## Fuera de alcance

- sincronizar records o conversaciones entre servidores;
- mover un workspace de un servidor a otro;
- ejecutar un agente sin una carpeta o repositorio determinado;
- compartir perfiles y credenciales entre usuarios del sistema operativo;
- búsqueda o bandeja de menciones federada;
- soportar varias cuentas simultáneas para la misma URL normalizada;
- eliminar la capa interna `Project` en este trabajo.

## Consecuencias

- Desktop se convierte en un cliente local multi-servidor real;
- un servidor local deja de confundirse con el modo **Este computador**;
- la incorporación gráfica reemplaza la necesidad de preparar una carpeta por
  terminal;
- el runtime puede coordinar simultáneamente agentes contra servidores
  independientes;
- perfiles, credenciales y bindings requieren migraciones locales y pruebas
  específicas en macOS y Windows;
- algunas APIs nuevas son necesarias para resolver repositorios sin revelar
  rutas locales;
- la interfaz debe comunicar constantemente qué información pertenece al
  computador y cuál al servidor activo.

## Plan de ejecución

La secuencia, contratos, migraciones, pruebas y criterios de salida están en
[Desktop multi-servidor — Plan de implementación](../plans/desktop-multi-server-implementation.md).
