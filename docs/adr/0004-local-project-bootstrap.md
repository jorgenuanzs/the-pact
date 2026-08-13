# ADR-0004 — Bootstrap local y vínculo con Pact Server

**Estado:** aceptado
**Fecha:** 7 de agosto de 2026
**Decisión que implementa:** cómo descubre Pact un proyecto local y cómo se
relaciona ese proyecto con una instalación personal o compartida.

## Contexto

Pact necesita ser reconocible desde la raíz de un repositorio, de forma similar
a como Git utiliza `.git`. Sin embargo, la configuración compartida del proyecto
y el estado privado de una máquina tienen ciclos de vida y riesgos distintos.

Además, una instalación de equipo no debe entregar credenciales de PostgreSQL a
cada persona o agente. Los clientes locales necesitan una autoridad común, pero
la base de datos debe permanecer como detalle privado de Pact Server.

## Decisión

Un proyecto inicializado por Pact contiene:

```text
repository/
├── pact.yaml       # contrato compartido y versionado con Git
├── .pact/          # vínculo y estado local, ignorado por Git
├── .git/
└── source...
```

### `pact.yaml`

Es un manifiesto declarativo compartido. La primera versión contiene:

- `apiVersion` y `kind` para evolucionar el formato;
- nombre del proyecto;
- modo de gobierno inicial;
- repositorios, rutas y referencia canónica.

No contiene endpoint personal, token, credenciales de base de datos, identidad
de nodo ni estado de una sesión. Las políticas compartidas se incorporarán aquí
o mediante rutas referenciadas por el manifiesto.

### `.pact/`

Es un directorio privado de la máquina, con permisos restringidos y excluido de
Git. En este corte contiene `config.json`, que declara la URL de Pact Server y
la versión del esquema local, y `node.json`, que identifica el nodo al abrir una
sesión mediante `pact agent run`. Más adelante podrá contener:

- certificado rotatorio y credenciales propias de Pact Node;
- cursor de sincronización y cola temporal sin conexión;
- sockets, locks y checkpoints;
- cachés regenerables;
- referencias a credenciales almacenadas en el llavero del sistema.

Los secretos no se escribirán en `.pact/`. Se obtendrán del llavero, de un
gestor de secretos o de una identidad de carga. La URL del servidor tampoco
puede incluir credenciales.

### `pact init`

El primer comando implementado:

1. encuentra la raíz Git desde la ruta indicada;
2. detecta la referencia de la rama actual cuando es posible;
3. crea `pact.yaml` sin sobrescribir un manifiesto existente;
4. crea `.pact/config.json` con permisos privados;
5. añade `.pact/` a `.gitignore` de forma idempotente;
6. permite seleccionar Pact Server mediante `--server`;
7. no almacena tokens ni credenciales de PostgreSQL.

`pact init` es deliberadamente seguro e idempotente. ADR-0005 amplía esta
decisión: `pact init` crea o recupera el proyecto remoto y `pact connect`
conecta explícitamente un checkout nuevo con un proyecto que ya existe.

## Topologías

### Uso personal

```text
Agentes y herramientas
        │ socket/API local
        ▼
Pact Node en la máquina
        │ HTTP/SSE hacia loopback
        ▼
Pact Server local ── red privada ── PostgreSQL local
```

Pact Server y PostgreSQL pueden ejecutarse con Docker Compose en la misma
máquina. Aunque todo sea local, Node habla con la API y no con PostgreSQL. Esto
mantiene la misma semántica que una instalación de equipo.

### Equipo o instalación remota

```text
Máquina de Ana ── Pact Node ──┐
Máquina de Luis ─ Pact Node ──┼── HTTPS ── Pact Server compartido
Agente de CI ──── Pact Node ──┘                    │
                                                   ├── PostgreSQL privado
                                                   ├── object storage
                                                   └── workers
```

Cada Node establece una conexión saliente con el mismo Pact Server. El servidor
autentica actores y nodos, coordina el estado común, registra eventos y publica
actualizaciones. PostgreSQL no se expone a las máquinas del equipo.

En una instalación compartida deberán existir TLS, OIDC o credenciales de nodo
rotatorias, autorización por organización/proyecto, roles PostgreSQL separados
y políticas de red. El token local actual no cumple esos requisitos y no debe
utilizarse como autenticación de producción.

## Autoridades

```text
Código e historial               Git
Declaración compartida           pact.yaml + Git
Estado local de una máquina      .pact/
Coordinación y eventos globales  Pact Server + PostgreSQL
Secretos                         keychain/Vault/identidad de carga
```

`.pact/` no es una base de datos global ni un mecanismo de bloqueo. Si se borra,
la máquina pierde su vínculo y caché local, pero el estado común continúa en el
servidor y puede reconstruirse después de autenticar y conectar de nuevo.

## Consecuencias

### Positivas

- cualquier herramienta puede descubrir que el repositorio utiliza Pact;
- la configuración compartida queda revisable en Git;
- cada máquina puede conectarse a una instalación diferente sin ensuciar el
  manifiesto;
- PostgreSQL y sus credenciales permanecen fuera de los clientes;
- el modo personal y el modo equipo utilizan la misma frontera de API.

### Costes y trabajo posterior

- el registro actual de nodo depende del CLI y falta Pact Node persistente;
- el endpoint local guardado no prueba que el servidor esté disponible;
- se necesitará una migración explícita cuando cambie el esquema local;
- la cola sin conexión y la sincronización por cursor pertenecen al futuro
  proceso Pact Node.
