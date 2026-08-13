# ADR-0008 — Adaptador MCP local

**Estado:** aceptado
**Fecha:** 13 de agosto de 2026

## Contexto

`pact agent run` permite envolver un proceso y Pact Node observa Git, pero un
agente todavía necesita invocar comandos manuales para consultar contexto. Los
clientes de IA no comparten una interfaz propia de PACT y no deben recibir el
token personal para resolver esa limitación.

Model Context Protocol (MCP) proporciona un contrato común para descubrir e
invocar herramientas. El transporte `stdio` permite que cada cliente inicie un
proceso local con sus mismos permisos, sin publicar otro puerto ni crear un
servicio remoto adicional.

## Decisión

El CLI incorpora:

```text
pact mcp serve --client <tipo> [--name <nombre>] [--path <checkout>]
```

El proceso carga el vínculo local del proyecto y la identidad personal guardada
para ese Pact Server. Abre una sesión de agente con `observe_git=true`, envía la
observación inicial, mantiene heartbeat y observa Git mientras el cliente MCP
conserve vivo el proceso. Al cerrarse `stdin`, recibir una señal o perder la
sesión, PACT intenta cerrar la sesión remota.

La implementación utiliza el SDK oficial de Go y `stdio`. `stdout` queda
reservado exclusivamente para mensajes MCP; advertencias y errores del SDK se
escriben en `stderr`.

El primer contrato ofrece:

```text
pact.project_context
pact.list_projects
pact.refresh_git_observation
```

`pact.project_context` es la entrada recomendada antes de comenzar un trabajo.
Entrega hechos operativos compartidos, no conversaciones privadas. La presencia
de una sesión no se presenta como prueba de edición; para eso conserva los
estados observados de Git definidos en ADR-0003.

## Frontera de seguridad y privacidad

- El token personal nunca se devuelve como contenido, argumento, variable de
  entorno generada ni log MCP.
- No se exponen la raíz local, rutas de workspaces, nombres ni contenidos de
  archivos.
- Las URLs remotas del repositorio se omiten porque podrían contener
  credenciales incrustadas.
- Los datos de eventos se filtran recursivamente; tokens, secretos,
  contraseñas, credenciales, claves privadas y URLs remotas se redactan.
- MCP no ofrece shell, acceso genérico a PostgreSQL, lectura arbitraria de
  archivos ni operaciones de infraestructura.
- La autorización continúa en Pact Server mediante el token personal y los
  roles de proyecto. MCP es un adaptador, no una autoridad paralela.

## Límites conscientes

- El transporte inicial es local por `stdio`; aún no existe un endpoint MCP
  remoto con OAuth ni transporte HTTP.
- Cada cliente debe registrar el comando en su propia configuración. PACT no
  modifica automáticamente configuraciones de terceros.
- El proceso sigue el ciclo de vida del cliente MCP y todavía no se instala como
  servicio residente del sistema operativo.
- El contexto es una vista estructurada de PACT; recursos MCP, prompts y
  suscripciones se añadirán solo cuando exista una necesidad de producto.
- Un agente conserva acceso normal al checkout según los permisos del sistema
  operativo. MCP informa y coordina, pero aún no impone scopes ni worktrees.

## Consecuencias

- Codex, Claude, Kimi u otro cliente compatible pueden consumir el mismo
  contexto sin integración específica con sus conversaciones.
- PACT conoce la sesión y la actividad Git durante el uso de MCP.
- La credencial permanece fuera del contexto del modelo.
- La siguiente limitación relevante ya no es acceso al contexto, sino coordinar
  intenciones, reservar scopes y aislar modificaciones concurrentes.
