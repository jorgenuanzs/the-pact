# ADR-0015 — GitHub App organizacional y proyectos multirrepositorio

**Estado:** aceptado

**Fecha:** 14 de agosto de 2026

## Contexto

ADR-0014 incorporó la primera proyección verificable del repositorio canónico,
pero dependía de una credencial estática y exponía únicamente el repositorio
raíz. Ese modelo no cubre productos que separan frontend, backend, aplicaciones
móviles, infraestructura o documentación, ni permite que un administrador
elija repositorios mediante la interfaz nativa de GitHub.

Una conexión de proveedor tampoco pertenece naturalmente a un solo proyecto.
La organización autoriza una instalación; cada proyecto consume un subconjunto
de sus repositorios.

## Decisión

PACT adopta una GitHub App como integración principal de producción. Las
instalaciones y los repositorios autorizados se almacenan en el esquema
`integrations` con alcance organizacional. Un proyecto enlaza uno o más de esos
repositorios mediante `coordination.repositories`.

Cada enlace declara:

- un propósito flexible, por ejemplo `frontend`, `backend`, `mobile`, `infra`
  o `docs`;
- si es necesario u opcional;
- si es el repositorio principal, expresado por
  `identity.projects.root_repository_id`.

El repositorio principal conserva la proyección
`identity.projects.canonical_revision` para compatibilidad. Cada repositorio,
incluidos los adicionales, conserva su rama y revisión verificadas en
`coordination.repository_provider_states`. Los Context Packs incluyen el
conjunto completo de repositorios y revisiones.

Los endpoints singulares de sincronización siguen representando el repositorio
principal. Los nuevos endpoints con `repository_id`, el CLI y MCP permiten
consultar o sincronizar cualquier repositorio del conjunto.

## Flujo de conexión

El botón **Conectar GitHub** está restringido a `owner` y `admin` de la
organización:

1. PACT genera un `state` aleatorio de un solo uso y guarda únicamente su
   SHA-256 durante diez minutos.
2. El navegador abre la página oficial de instalación de GitHub, donde el
   usuario elige una cuenta y todos o algunos repositorios.
3. GitHub vuelve al Setup URL con `installation_id` y `state`. PACT asocia ese
   identificador al intento pendiente, pero todavía no confía en él.
4. PACT inicia el flujo web de autorización de usuario con el mismo `state` y
   PKCE S256. El verificador se deriva mediante HMAC y no se persiste.
5. Tras intercambiar el código, PACT consulta la instalación con el token de
   usuario para demostrar que el usuario tiene acceso a ella.
6. PACT descarta el token de usuario, consulta la instalación como GitHub App y
   guarda solamente metadatos y repositorios autorizados.

La opción de GitHub **Request user authorization during installation** debe
permanecer desactivada. Tanto el Callback URL como el Setup URL apuntan a
`/v1/integrations/github/callback`.

## Credenciales y webhooks

La clave privada, el client secret y el webhook secret viven únicamente en el
entorno secreto de Pact Server. PostgreSQL nunca almacena tokens.

Para leer un repositorio privado, PACT firma un JWT de aplicación y solicita un
token de instalación limitado al identificador exacto de ese repositorio y a
los permisos `metadata:read` y `contents:read`. El token dura como máximo una
hora, se conserva solo en memoria y deja de reutilizarse un minuto antes de su
expiración.

Los webhooks se validan con HMAC-SHA256 antes de procesarse. Los identificadores
de entrega se registran para hacer el procesamiento idempotente. Los eventos
`installation` e `installation_repositories` actualizan suspensiones,
eliminaciones y cambios en la selección de repositorios.

`PACT_GITHUB_TOKEN` se conserva como alternativa explícita para desarrollo o
GitHub Enterprise Server. Si existe una instalación válida, el token dinámico
de la App tiene prioridad.

## Consecuencias

- una conexión sirve para varios proyectos de la misma organización;
- GitHub sigue controlando qué repositorios son visibles para PACT;
- un proyecto puede coordinar trabajo y contexto a través de varios
  repositorios sin fingir que existe un único hash global;
- cambiar el repositorio principal es compatible con clientes anteriores;
- los scopes existentes ya incluyen `repository_id`, por lo que las reservas
  de rutas continúan separadas por repositorio;
- PACT todavía no escribe código, crea ramas, fusiona cambios ni gobierna
  pushes mediante esta integración de solo lectura.
