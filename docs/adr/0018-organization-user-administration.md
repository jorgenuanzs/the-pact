# ADR-0018 — Administración organizacional de usuarios y permisos

**Estado:** aceptado  
**Fecha:** 17 de agosto de 2026  
**Complementa:** ADR-0006 y ADR-0017

## Contexto

Las cuentas locales resolvieron la autenticación humana y el flujo de
autorización de dispositivos, pero faltaba una superficie operativa para
incorporar personas, cambiar sus responsabilidades, limitar proyectos y cortar
su acceso sin intervenir directamente en PostgreSQL.

Pact necesita conservar trazabilidad incluso cuando una persona deja la
organización. Por eso eliminar físicamente una identidad no es una operación
administrativa segura: rompería la atribución histórica de agentes, trabajo,
mensajes y eventos.

## Decisión

El backoffice incluye un directorio organizacional disponible únicamente para
`owner` y `admin`. Permite:

- listar cuentas activas y desactivadas;
- editar nombre, correo y usuario;
- asignar roles `owner`, `admin` y `member`;
- asignar a miembros roles directos por proyecto;
- crear y revocar invitaciones de cuenta de un solo uso;
- revocar sesiones web, dispositivos y sesiones activas de agentes
  patrocinados;
- desactivar y reactivar cuentas;
- consultar una auditoría durable de esas acciones.

La baja es lógica. Una cuenta desactivada conserva todas sus referencias
históricas, pero no puede iniciar sesión y pierde inmediatamente sus sesiones y
dispositivos vigentes.

## Límites de autorización

- Un `owner` puede administrar cualquier otra cuenta.
- Un `admin` puede administrar solamente cuentas `member` e invitar nuevos
  miembros.
- Nadie puede desactivarse, revocar sus propias sesiones desde esta superficie
  ni cambiar su propio rol.
- El último `owner` activo no puede ser desactivado ni degradado.
- `owner` y `admin` tienen acceso global a proyectos; los permisos directos se
  usan solamente para `member`.
- Las operaciones administrativas requieren una sesión web interactiva y
  protección CSRF. Una credencial de dispositivo o un agente no puede ejecutar
  estas APIs.

Las comprobaciones viven tanto en el servicio como dentro de las transacciones
de PostgreSQL. Un bloqueo asesor por organización serializa cambios sensibles,
incluida la protección del último propietario.

## Invitaciones

Crear un usuario significa emitir una invitación. Pact no genera ni comunica
contraseñas temporales: devuelve una sola vez un enlace secreto y la persona
elige su contraseña al registrarse. Una invitación puede conceder un rol de
organización y, si ese rol es `member`, un permiso inicial opcional de proyecto.

Solo puede existir una invitación pendiente por correo dentro de la
organización. El secreto se almacena como digest y no aparece en auditoría.

## Consecuencias

La API y el backoffice forman un plano de control administrativo verificable,
pero no reemplazan el modelo de acceso de los proyectos. Las futuras
integraciones con SSO, SCIM o directorios externos deberán crear o vincular las
mismas identidades y respetar estas invariantes, no introducir una segunda
fuente de autorización.
