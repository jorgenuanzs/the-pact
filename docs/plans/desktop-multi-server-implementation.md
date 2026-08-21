# PACT Desktop multi-servidor — Plan de implementación

**Estado:** en ejecución
**ADR:** [ADR-0019](../adr/0019-desktop-multi-server-and-folder-bindings.md)  
**Fecha:** 21 de agosto de 2026

**Progreso:** Hitos 1, 2 y 3 implementados; Hito 4 pendiente.

## Resultado esperado

Una persona puede instalar PACT Desktop una sola vez, autorizar varios PACT
Servers y vincular carpetas locales con workspaces y repositorios de cualquiera
de ellos. Codex y Claude pueden trabajar simultáneamente en carpetas asociadas
a servidores distintos sin depender del servidor que esté abierto en la UI.

La experiencia debe distinguir siempre:

- **Este computador:** perfiles, rutas, clientes y runtime locales;
- **Servidor activo:** workspaces, personas, contexto, conversaciones y
  administración remota;
- **Servidor local:** instalación de PACT Server administrada por Docker, que
  aparece como un perfil más.

## Línea base

Antes de comenzar:

- `internal/userconfig` guarda una única URL y credencial de dispositivo;
- Desktop bloquea la aplicación hasta autorizar ese servidor;
- `.pact/config.json` v1 guarda `server_url` y `project_id`;
- el registro local Desktop v1 guarda carpeta, servidor, proyecto y clientes;
- `ConnectLocalAgent` solo habilita MCP y exige un binding previo;
- `pact mcp serve` busca la única credencial global;
- el modal actual no puede crear el binding ni cambiar de perfil;
- PACT Server no recibe rutas locales, invariante que debe mantenerse.

## Arquitectura de destino

```text
┌──────────────────────── PACT Desktop ────────────────────────┐
│                                                              │
│  Profile Registry ──→ Credential Store                       │
│        │                  Keychain / Credential Manager       │
│        │                                                     │
│        ├── Nuanzs Cloud                                      │
│        ├── Client A Cloud                                    │
│        └── Local PACT ──→ Docker-managed PACT Server         │
│                                                              │
│  Folder Registry                                             │
│        ├── checkout A → profile A / workspace / repository   │
│        └── checkout B → profile B / workspace / repository   │
│                                                              │
│  Local Runtime                                               │
│        └── binding → profile → credential → PACT Server      │
└──────────────────────────────────────────────────────────────┘
```

## Principios de entrega

1. Cada cambio de persistencia incluye migración y prueba de rollback o
   reintento.
2. CLI y Desktop comparten los paquetes de perfiles, credenciales y bindings;
   no mantienen implementaciones paralelas.
3. Ningún PR deja MCP leyendo una credencial diferente de la indicada por el
   binding.
4. Las nuevas superficies se activan solo cuando el recorrido vertical está
   completo; se evita publicar una UI que prometa multi-servidor mientras el
   runtime siga siendo global.
5. Todas las operaciones que escriben archivos son atómicas e idempotentes.
6. Cada PR con comportamiento visible actualiza `CHANGELOG.md` bajo
   `Unreleased`.

## Hito 1 — Registro de perfiles y almacén seguro

**Estado:** implementado el 21 de agosto de 2026.

### Paquetes

Crear una frontera compartida, por ejemplo:

```text
internal/serverprofile
internal/credentialstore
```

`serverprofile.Registry` debe ofrecer:

- `List`;
- `Get`;
- `FindByURL`;
- `UpsertAuthorized`;
- `SetActive`;
- `Remove`;
- migración desde `userconfig` v2.

`credentialstore.Store` debe ofrecer:

- `Put(profileID, secret)`;
- `Get(profileID)`;
- `Delete(profileID)`;
- `Exists(profileID)`.

### Persistencia

El archivo local nuevo contiene metadatos, nunca secretos:

```json
{
  "schema_version": 3,
  "active_profile_id": "…",
  "profiles": [
    {
      "id": "…",
      "label": "Nuanzs",
      "server_url": "https://pact.nuanzs.com",
      "kind": "remote",
      "principal_id": "…",
      "principal_label": "jorge@nuanzs.com",
      "credential_ref": "pact/server/…"
    }
  ]
}
```

### Migración

1. Leer v2 sin modificarla.
2. Crear ID estable y escribir la credencial en el almacén nativo.
3. Leerla nuevamente y comprobar que el valor coincide; los flujos nuevos ya
   validan el principal contra `/v1/me` antes de guardar y una configuración
   migrada lo revalida en su primera operación remota.
4. Escribir el registro v3 mediante rename atómico.
5. Sustituir el archivo anterior sin conservar ninguna copia que contenga el
   secreto.
6. El registro v3 actúa como marcador durable de migración.

Si cualquier paso falla, v2 permanece utilizable y la siguiente apertura
reanuda el proceso.

### Criterios de salida

- macOS y Windows guardan credenciales en sus almacenes nativos;
- dos perfiles conservan credenciales diferentes;
- la misma URL se reautoriza sin duplicarse;
- una credencial nunca aparece en JSON, logs ni bindings;
- migrar una configuración actual conserva la sesión.

## Hito 2 — CLI multi-servidor compatible

**Estado:** implementado el 21 de agosto de 2026.

Actualizar los comandos:

```text
pact login --server URL [--name NAME]
pact servers list
pact servers use PROFILE_OR_URL
pact servers remove PROFILE_OR_URL
pact logout --server URL
```

Comportamiento:

- `login` añade o reautoriza un perfil;
- `use` cambia solo la preferencia para comandos sin carpeta;
- comandos ejecutados dentro de una carpeta priorizan su `server_url`;
- `init` exige perfil explícito si no hay perfil activo;
- `connect` puede recibir `--server`, pero nunca reemplaza otros perfiles;
- `logout` revoca y retira un perfil concreto;
- retirar el perfil activo selecciona otro perfil disponible o ninguno.

### Criterios de salida

- dos terminales pueden operar simultáneamente contra servidores distintos;
- `pact status` indica servidor, workspace y repositorio de la carpeta;
- un comando nunca cae silenciosamente al perfil activo si existe un binding;
- los mensajes de error nombran el perfil o URL que falta autorizar.

## Hito 3 — Binding de carpeta v2

**Estado:** implementado el 21 de agosto de 2026.

Ampliar `internal/localproject.Binding` con:

- `WorkspaceID`;
- `RepositoryID`;
- `ProjectID` transitorio;
- fingerprint del remoto Git normalizado.

Implementar:

- lectura compatible de esquema v1;
- escritura exclusiva de esquema v2;
- migración diferida cuando el servidor esté fuera de línea;
- validación de que el repositorio pertenece al workspace indicado;
- operación explícita e idempotente de rebinding;
- rotación de `node.json` cuando cambia de servidor.

La ruta local sigue fuera del archivo compartido `pact.yaml` y fuera del
servidor.

### Criterios de salida

- v1 continúa abriendo;
- v2 identifica inequívocamente el repositorio dentro del workspace;
- un checkout no admite dos servidores simultáneos;
- un worktree separado puede usar otro servidor;
- escrituras interrumpidas no corrompen el binding anterior.

## Hito 4 — Resolución autorizada de repositorios

Añadir un contrato servidor para resolver un remoto Git sin crear estado local.
Nombre propuesto:

```text
POST /v1/repository-bindings/resolve
```

Entrada:

```json
{
  "remote_url": "https://github.com/example/repository",
  "workspace_id": "opcional"
}
```

Salida:

```json
{
  "matches": [
    {
      "workspace_id": "…",
      "workspace_name": "Footfall",
      "project_id": "…",
      "repository_id": "…",
      "repository_name": "footfall-api",
      "match": "exact",
      "permission": "maintainer"
    }
  ]
}
```

Reglas:

- POST evita incluir remotos potencialmente sensibles en query strings y
  access logs;
- cliente y servidor eliminan userinfo, tokens y sufijos no significativos;
- solo se devuelven entidades visibles para el principal;
- una coincidencia inaccesible no revela nombre, organización ni existencia;
- el endpoint no recibe `root`, nombre de usuario ni rutas locales;
- workspace y repository se validan en la misma organización.

El flujo de adjuntar un repositorio autorizado por GitHub puede reutilizar las
APIs existentes. Crear un workspace o adjuntar un repositorio exige permisos
de administración y confirmación independiente.

### Criterios de salida

- SSH, HTTPS y `ssh://` del mismo remoto producen la misma coincidencia;
- URLs con credenciales son rechazadas o limpiadas antes de loguearse;
- no hay filtración de repositorios sin acceso;
- se cubren coincidencia única, múltiple y nula.

## Hito 5 — Runtime y MCP multi-perfil

Reemplazar `loginForServer` por resolución estricta:

```text
binding.server_url
    → serverprofile.FindByURL
    → credentialstore.Get
    → /v1/me y autorización del recurso
```

Actualizar `pact mcp serve`, `pact node run`, `pact agent run` y cualquier
observador residente.

Reglas:

- con binding, nunca usar `active_profile_id` como fallback;
- sin perfil para la URL, devolver una instrucción de autorización concreta;
- una autorización vencida afecta solo ese perfil;
- variables de entorno continúan indicando el servidor del binding;
- la configuración MCP no contiene secretos ni profile IDs necesarios para
  reconstruir la conexión;
- sesiones simultáneas mantienen streams, heartbeats y cierres independientes.

### Criterios de salida

- Codex en carpeta A publica presencia en servidor A;
- Claude en carpeta B publica presencia en servidor B al mismo tiempo;
- cambiar la UI a servidor C no afecta esas sesiones;
- una prueba adversarial impide usar la credencial B para un binding A;
- cerrar una sesión no cancela streams de otro servidor.

## Hito 6 — API nativa de Desktop y registro local v2

Desktop debe exponer al frontend operaciones orientadas a dominio:

- listar, añadir, reautorizar, renombrar, seleccionar y retirar perfiles;
- consultar salud de perfiles sin bloquear toda la ventana;
- inspeccionar una carpeta;
- resolver destinos en un perfil;
- crear, cambiar o reparar un binding;
- habilitar o deshabilitar clientes por carpeta;
- listar carpetas de todos los perfiles;
- administrar el servidor local.

El registro de carpetas deja de duplicar autoridad. Se considera un índice de
rutas elegidas; el binding dentro de `.pact/` y la respuesta autorizada del
servidor determinan el destino.

Los métodos nativos no devuelven credenciales al frontend. Todas las llamadas
que requieren autenticación se ejecutan en Go.

### Criterios de salida

- la aplicación puede abrir sin perfiles;
- un servidor caído aparece degradado sin bloquear perfiles sanos;
- carpetas desaparecidas se pueden retirar del índice sin tocar Git;
- retirar un perfil con carpetas exige una decisión explícita;
- el frontend no recibe secretos ni referencias canjeables.

## Hito 7 — Separación visual entre computador y servidor

### Modo local

```text
Este computador
├── Resumen local
├── Conexiones PACT
├── Carpetas
├── Clientes de IA
└── Servidor local
```

**Resumen local** muestra cantidad de perfiles, carpetas, clientes y estado del
runtime. **Conexiones PACT** permite añadir, reautorizar, cambiar nombre, abrir
administración y retirar perfiles.

### Modo servidor

- muestra nombre y URL del perfil activo;
- lista únicamente workspaces de ese servidor;
- la cuenta y **Acceso y seguridad** pertenecen a ese perfil;
- las menciones pertenecen a ese perfil;
- cambiar servidor cancela suscripciones de la vista anterior y abre las del
  nuevo, sin tocar runtimes locales activos.

### Onboarding

Desktop ya no bloquea toda la aplicación hasta conectar un servidor. La
primera pantalla local ofrece:

- **Añadir PACT Server remoto**;
- **Crear PACT Server local**;
- explicación de que se pueden añadir más después.

### Criterios de salida

- siempre se distingue visualmente **Este computador** del servidor activo;
- no se muestran workspaces de dos servidores en la misma lista;
- el perfil activo persiste entre aperturas;
- un servidor sin conexión no deja la aplicación en blanco;
- accesibilidad por teclado y lectores de pantalla cubre el selector.

## Hito 8 — Nuevo flujo “Añadir carpeta a PACT”

### Paso 1 — Carpeta

La persona selecciona una carpeta. Desktop muestra:

- nombre y ruta local;
- remoto Git normalizado;
- rama actual;
- binding existente, si lo hay;
- problemas accionables: no Git, remoto ausente o archivos inválidos.

### Paso 2 — Servidor

- binding existente: se muestra su perfil y se ofrece autorizarlo si falta;
- carpeta nueva: se preselecciona el perfil desde el cual se abrió el flujo;
- siempre se puede escoger otro perfil guardado o añadir uno nuevo;
- un perfil no disponible puede reintentarse sin perder la carpeta elegida.

### Paso 3 — Workspace y repositorio

- coincidencia exacta única: preselección y explicación;
- varias coincidencias: selector agrupado por workspace;
- ninguna: selección manual;
- rol suficiente: acciones separadas para crear workspace o adjuntar repo;
- sin permiso: mensaje de solicitud de acceso, sin inventar un destino.

### Paso 4 — Clientes

Casillas independientes para Codex y Claude. Se muestran detección, estado
actual y si cada cliente necesita reiniciarse.

### Paso 5 — Confirmación

Resumen completo:

```text
/dev/footfall-api
→ Nuanzs Cloud
→ Footfall
→ footfall-api
→ Codex + Claude
```

El CTA se adapta:

- **Vincular carpeta**;
- **Vincular carpeta y conectar Codex**;
- **Conectar Claude** si el binding ya existe.

### Estados parciales

Orden de escritura:

1. verificar perfil, permiso y destino;
2. escribir binding atómicamente;
3. habilitar cada cliente idempotentemente;
4. actualizar el índice local;
5. mostrar resultado por cliente.

Si falla un cliente en el paso 3, el binding permanece válido y se ofrece
**Reintentar Claude**. Nunca se muestra el error interno sin traducción.

### Criterios de salida

- una carpeta nueva se incorpora sin abrir una terminal;
- el flujo puede elegir cualquiera de los perfiles guardados;
- una carpeta ya vinculada no solicita nuevamente workspace y repositorio;
- un binding a un perfil ausente conduce a autorización, no a un callejón sin
  salida;
- selección y reintentos conservan los pasos ya completados;
- el modal funciona con teclado y anuncia errores junto al paso correspondiente.

## Hito 9 — Rebinding, retiro y recuperación

### Cambiar vínculo

- acción separada del modal de clientes;
- presenta origen y destino;
- detecta sesiones locales activas;
- requiere confirmación cuando cambia de servidor;
- rota identidad de nodo y revalida clientes;
- no mueve código ni modifica el remoto Git.

### Retirar carpeta de Desktop

Retira la ruta del índice local. Por defecto conserva `.pact/` y la
configuración MCP. Una segunda acción explícita puede deshabilitar clientes y
eliminar el binding local, sin borrar el repositorio.

### Retirar perfil

Opciones cuando tiene carpetas:

- cancelar;
- conservar bindings huérfanos y retirar la credencial;
- cambiar cada carpeta a otro destino mediante rebinding.

No existe cambio masivo implícito de servidor.

### Recuperación

- reconstruir el índice seleccionando una carpeta ya vinculada;
- reautorizar un perfil sin reescribir bindings;
- reparar un `.pact/config.json` legacy consultando su `project_id`;
- exportar diagnóstico redactado sin rutas completas ni secretos.

## Hito 10 — Migración, documentación y release

### Pruebas de migración

Cubrir:

- instalación actual con un servidor y varias carpetas;
- configuración v2 con autorización válida;
- autorización expirada;
- servidor fuera de línea durante migración;
- carpeta v1 disponible y carpeta desaparecida;
- servidor local instalado, detenido y actualizado;
- reanudación después de interrumpir cada paso de escritura.

### Documentación

Actualizar:

- README y guía de instalación;
- onboarding Desktop remoto y local;
- comandos CLI multi-servidor;
- recuperación y retiro de perfiles;
- explicación de Desktop, Server, runtime y binding;
- modelo de seguridad de credenciales.

### Publicación

No publicar la UX multi-servidor hasta completar los hitos 1–8 y el recorrido
vertical. Antes del release:

1. ejecutar migraciones sobre fixtures de versiones anteriores;
2. probar dos servidores remotos y uno local en macOS;
3. repetir el recorrido soportado en Windows;
4. verificar instalador y actualización desde la versión estable anterior;
5. revisar `CHANGELOG.md` y convertir `Unreleased` en la nueva versión;
6. publicar Desktop, CLI e imagen de PACT Server juntos;
7. desplegar producción por separado después de validar los artefactos;
8. conservar una guía de recuperación de la versión anterior.

## Secuencia propuesta de pull requests

| PR | Alcance | Puede publicarse aisladamente |
|---|---|---|
| 1 | ADR, plan y vocabulario | Sí, documentación |
| 2 | `CredentialStore`, perfiles y migración v2→v3 | No activar UI nueva |
| 3 | CLI multi-perfil | Sí, manteniendo compatibilidad |
| 4 | Binding v2 y migración local | Sí, lectura v1 obligatoria |
| 5 | Endpoint de resolución de repositorios | Sí, API aditiva |
| 6 | Runtime MCP resuelto por binding | Sí, después de perfiles |
| 7 | Bridge Desktop y registro local v2 | No activar UI nueva |
| 8 | Navegación local/remota y gestor de perfiles | Junto al PR 9 o con flag |
| 9 | Nuevo wizard de carpetas y estados parciales | Completa el recorrido |
| 10 | Rebinding, retiro, recuperación y E2E | Candidato a release |

Cada PR debe ser pequeño en su frontera, pero el release se acumula hasta que
el recorrido completo esté listo.

## Matriz mínima de pruebas

| Escenario | Unitarias | Integración | Desktop E2E |
|---|---:|---:|---:|
| Migrar perfil único | Sí | Sí | Sí |
| Añadir tres perfiles | Sí | Sí | Sí |
| Reautorizar misma URL | Sí | Sí | Sí |
| Keychain / Credential Manager | Sí | Sí | Sí |
| Resolver remoto SSH/HTTPS | Sí | Sí | No |
| Binding nuevo | Sí | Sí | Sí |
| Binding legacy offline | Sí | Sí | Sí |
| Codex A + Claude B simultáneos | Sí | Sí | Sí |
| Servidor A caído, B operativo | Sí | Sí | Sí |
| Rebinding con sesión activa | Sí | Sí | Sí |
| Retirar perfil con carpetas | Sí | Sí | Sí |
| Ausencia de secretos en archivos/logs | Sí | Sí | Sí |
| Actualización desde versión estable | No | Sí | Sí |

## Observabilidad

Desktop debe registrar eventos locales estructurados, redactados y rotables:

- `profile.authorization.started|completed|failed`;
- `profile.health.changed`;
- `folder.inspected|bound|rebound|removed`;
- `client.enabled|failed|disabled`;
- `runtime.started|connection_failed|stopped`;
- `local_server.installed|started|stopped|upgraded`.

Los eventos incluyen IDs locales y códigos de error. Rutas completas, remotos
con userinfo, correos y credenciales no aparecen por defecto.

## Riesgos y mitigaciones

### Migración de secretos

**Riesgo:** perder una sesión válida o conservar el token antiguo.  
**Mitigación:** copia primero, verifica lectura y `/v1/me`, escribe metadatos y
solo entonces elimina el secreto anterior.

### Resolución incorrecta de repositorio

**Riesgo:** vincular un checkout con un repositorio homónimo.  
**Mitigación:** comparar identidad canónica completa, mostrar destino y exigir
confirmación en coincidencias no exactas.

### Confusión entre perfil activo y binding

**Riesgo:** enviar actividad al servidor visible en vez del servidor de la
carpeta.  
**Mitigación:** el runtime ignora el perfil activo cuando existe binding y las
pruebas adversariales fijan esta invariante.

### Diferencias entre almacenes nativos

**Riesgo:** comportamiento distinto en macOS y Windows.  
**Mitigación:** interfaz común, suite contractual y E2E en ambos runners.

### Estado parcialmente configurado

**Riesgo:** binding creado pero cliente MCP fallido.  
**Mitigación:** resultado por etapa, operaciones idempotentes y reintento
visible sin deshacer un binding válido.

### Crecimiento del alcance

**Riesgo:** intentar federar datos o eliminar `Project` durante el mismo
trabajo.  
**Mitigación:** respetar los límites del ADR y abrir decisiones separadas para
federación o simplificación del dominio servidor.

## Criterios finales de aceptación

El trabajo se considera completo cuando:

1. Desktop abre y administra estado local sin requerir un servidor activo.
2. Se pueden guardar al menos dos servidores remotos y uno local.
3. Cada perfil conserva identidad, salud y credencial independiente.
4. Cambiar el servidor visible no altera agentes en ejecución.
5. Una carpeta se vincula desde la UI con servidor, workspace y repositorio.
6. Codex y Claude pueden habilitarse juntos o por separado en cada carpeta.
7. Dos carpetas pueden operar simultáneamente contra servidores diferentes.
8. Un checkout no admite destinos ambiguos.
9. Los datos remotos nunca contienen rutas locales ni credenciales.
10. La configuración estable anterior migra sin intervención manual.
11. Un servidor caído no bloquea el control local ni otros perfiles.
12. Rebinding y retiro requieren decisiones explícitas y recuperables.
13. macOS y Windows superan el recorrido de instalación, migración y uso.
14. Las notas de release explican el nuevo modelo y sus límites.
