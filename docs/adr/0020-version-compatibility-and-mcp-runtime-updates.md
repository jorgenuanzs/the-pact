# ADR-0020 — Versiones, compatibilidad y actualización del runtime MCP

**Estado:** aceptado  
**Fecha:** 24 de agosto de 2026  
**Complementa:** ADR-0019

## Contexto

PACT se distribuye como varias piezas que no siempre cambian al mismo tiempo:

- PACT Desktop;
- PACT Server, instalado en una URL remota o administrado localmente con
  Docker;
- el runtime que Codex y Claude ejecutan mediante MCP;
- las configuraciones MCP privadas de cada checkout.

Desktop ya podía actualizarse mediante releases firmados y PACT Server ya
publicaba imágenes versionadas. Sin embargo, las configuraciones MCP guardaban
la ruta absoluta de un runtime identificado por el hash de su contenido. Una
actualización extraía un runtime nuevo, pero los checkouts existentes seguían
ejecutando el anterior.

Tampoco existía una presentación conjunta de la versión ejecutada por cada
PACT Server autorizado ni un contrato explícito para decidir si dos versiones
podían comunicarse.

## Decisión

### 1. PACT diferencia versión de producto, build y protocolo

- La **versión de producto** utiliza SemVer, por ejemplo `0.17.0`.
- El **build** conserva el commit y la fecha exactos. Un despliegue alojado
  puede utilizar además un identificador inmutable basado en fecha y commit.
- La **versión de protocolo** es un entero independiente de SemVer. Cada build
  anuncia el protocolo máximo y el protocolo mínimo que acepta.

`GET /version` devuelve esas cinco propiedades sin requerir autenticación:

```json
{
  "data": {
    "version": "0.17.0",
    "commit": "…",
    "date": "…",
  "protocol_version": 2,
  "min_protocol_version": 1
  }
}
```

Las versiones anteriores que no anuncien protocolo se presentan como
**heredadas**, no como incompatibles. Una incompatibilidad solo se declara
cuando los intervalos publicados no se superponen.

Las capacidades que cambian una operación concreta se habilitan además por su
versión mínima. Por ejemplo, registrar un repositorio directamente dentro de
un workspace requiere protocolo 2; Desktop no intenta el flujo histórico
contra un servidor heredado porque este creaba un workspace intermedio.

### 2. Desktop inspecciona todos sus perfiles de servidor

La superficie **Este computador → Conexiones PACT** consulta `/version` para
cada perfil autorizado y muestra:

- disponibilidad;
- versión y commit;
- compatibilidad con Desktop;
- disponibilidad de una actualización conocida.

Esta inspección no concede capacidad de despliegue. Una autorización de
usuario permite consumir PACT Server, pero no modificar la VM, el clúster o el
Docker remoto.

### 3. Los servidores locales tienen actualización administrada

Un PACT Server instalado por Desktop mantiene una imagen Docker fijada. La
acción **Actualizar**:

1. crea un respaldo de PostgreSQL;
2. fija la imagen correspondiente al release de Desktop;
3. descarga la imagen;
4. ejecuta migraciones;
5. recrea los servicios y espera sus health checks.

Actualizar Desktop no ejecuta esta operación de forma silenciosa. El servidor
puede contener datos importantes y su actualización continúa siendo una
acción explícita y auditable.

Los servidores autohospedados se actualizan con su propio pipeline o cambiando
la imagen fijada en Compose. PACT podrá incorporar más adelante un agente de
operaciones opcional, pero nunca inferirá acceso a infraestructura a partir de
una sesión normal de usuario.

### 4. MCP utiliza un launcher estable

Las configuraciones administradas de Codex y Claude ya no apuntan directamente
al runtime con hash. Apuntan a una ruta estable por usuario:

```text
macOS:   ~/Library/Application Support/Pact/bin/pact-mcp
Windows: %APPDATA%\Pact\bin\pact-mcp.exe
```

Desktop conserva el runtime real en:

```text
Pact/runtime/<sha256-prefix>/pact-local[.exe]
```

El archivo privado `Pact/runtime/active` selecciona la versión. El launcher:

1. valida estrictamente el identificador;
2. comprueba el SHA-256 del runtime;
3. ejecuta el runtime activo conservando argumentos, directorio y stdio.

Desktop cambia el puntero de forma atómica al instalar una versión nueva. Un
chat ya abierto conserva el proceso que inició; un chat nuevo utiliza el
runtime actual sin modificar otra vez `.codex/config.toml` o `.mcp.json`.

### 5. Migración

Al leer el estado local, Desktop vuelve a renderizar únicamente los bloques
MCP que PACT administra en las carpetas recordadas. Las entradas externas se
preservan y los conflictos continúan fallando de forma segura. Si una entrada
cambia, la interfaz indica que se debe abrir un chat nuevo.

No se guardan credenciales en el launcher, en el puntero activo ni en las
configuraciones MCP. El runtime resuelve el perfil autorizado a partir del
binding de la carpeta y del almacén seguro del sistema operativo.

## Consecuencias

- Una actualización futura de Desktop también entrega el runtime MCP nuevo.
- Las configuraciones por carpeta permanecen estables entre releases.
- Un servidor remoto puede evolucionar de forma independiente mientras su
  intervalo de protocolo siga siendo compatible.
- PACT distingue claramente **hay una versión nueva** de **debe actualizarse
  ahora**.
- Los clientes MCP en ejecución requieren un chat nuevo para adoptar un
  runtime recién activado.
