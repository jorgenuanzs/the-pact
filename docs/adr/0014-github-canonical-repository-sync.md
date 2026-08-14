# ADR-0014 — Estado canónico verificado desde GitHub

**Estado:** aceptado

**Fecha:** 14 de agosto de 2026

**Extensión:** ADR-0015 implementa la GitHub App, los webhooks y los proyectos
multirrepositorio previstos por esta decisión.

## Contexto

PACT recibía observaciones privadas desde cada checkout, pero
`identity.projects.canonical_revision` solo reflejaba el valor conocido durante
el onboarding. Una observación local no demuestra cuál es el commit actual de
la rama por defecto en el proveedor remoto: puede haber varios clones, pushes
externos o un cambio de rama canónica.

Git continúa siendo la fuente de verdad para archivos e historial. PACT
necesita conservar una proyección verificable de esa fuente para comparar
intenciones, Context Packs y trabajo local con un punto canónico común.

## Decisión

PACT incorpora un adaptador de proveedor para GitHub. El servidor consulta:

1. los metadatos del repositorio para conocer nombre canónico, visibilidad y
   rama por defecto;
2. el commit actual de esa rama.

El resultado se guarda en `coordination.repository_provider_states`. La tabla
contiene solo estado operativo: proveedor, repositorio, rama, commit,
visibilidad, timestamps, código de error sanitizado y versión. Nunca almacena
tokens ni cuerpos de error del proveedor.

Una sincronización correcta actualiza atómicamente:

- el estado observado del proveedor;
- `coordination.repositories.default_branch`;
- `identity.projects.canonical_revision`;
- el evento durable `pact.repository.canonical_synced.v1`, solo cuando cambia
  un valor material.

Un cambio de éxito a error produce `pact.repository.sync_failed.v1`. Repetir el
mismo error solo refresca `last_attempt_at`, evitando ruido periódico en el
event log. El comando HTTP es idempotente y requiere rol `maintainer`.

La verificación se puede iniciar mediante HTTP, CLI, MCP o un polling opcional
configurado con `PACT_GITHUB_SYNC_INTERVAL`. El polling está deshabilitado por
defecto. Los repositorios públicos funcionan sin credencial; los privados
requieren `PACT_GITHUB_TOKEN` en el entorno del proceso.

## Seguridad

- la URL remota se analiza y no puede contener credenciales, query ni fragment;
- el token se envía únicamente en `Authorization: Bearer` hacia el host API
  configurado y nunca se registra ni devuelve;
- los redirects a otro host son rechazados;
- las respuestas tienen tamaño limitado y los errores externos se reducen a
  códigos estables;
- el dashboard muestra estado y códigos, no detalles potencialmente sensibles.

Para una instalación de equipo, una GitHub App con acceso solo a los
repositorios seleccionados es el mecanismo objetivo. Un token estático permite
el bootstrap, pero no ofrece instalación por repositorio, atribución de app ni
rotación automática de tokens de una hora.

## Consecuencias y evolución

- los agentes pueden distinguir su HEAD local de la revisión remota confirmada;
- cambiar la rama por defecto en GitHub se refleja sin editar PACT manualmente;
- una caída de GitHub no elimina el último estado correcto: cambia el estado a
  `failed` y conserva la última revisión conocida;
- la integración actual es de solo lectura y no gobierna merges ni pushes;
- webhooks y credenciales dinámicas de GitHub App reemplazarán progresivamente
  el polling y el token estático, manteniendo el mismo modelo persistido.
