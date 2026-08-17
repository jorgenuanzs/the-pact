# ADR-0006 — Acceso personal, invitaciones y roles de proyecto

**Estado:** reemplazado por [ADR-0017](0017-local-accounts-web-sessions-and-device-authorization.md)
**Fecha:** 13 de agosto de 2026

## Contexto

La primera instalación utilizaba una sola credencial bootstrap para todas las
personas y agentes. Esa credencial permitía comprobar el servidor, pero impedía
atribuir acciones, retirar el acceso a una persona concreta y aplicar mínimos
privilegios. Compartirla con un colaborador equivalía a entregarle control
administrativo de toda la instalación.

## Decisión

La credencial configurada mediante `PACT_LOCAL_API_TOKEN` se conserva como
acceso de recuperación. El uso ordinario se realiza con principales humanos y
tokens personales opacos.

El flujo inicial es:

```text
owner                           colaborador
  │ pact invite create              │
  ├──── secreto de un solo uso ─────► pact join
  │                                  │
  │                                  ├─ token personal
  │                                  ├─ pact connect
  │                                  └─ pact agent run
```

Una invitación contiene proyecto, correo, rol y vencimiento. Su secreto tiene
256 bits aleatorios, se entrega una sola vez y PostgreSQL conserva únicamente
su SHA-256. Caduca en 24 horas por defecto y nunca puede superar siete días.
Aceptar la invitación la consume atómicamente, crea o recupera el principal,
asigna las membresías y emite un token personal con 30 días de vigencia.

Los tokens personales:

- utilizan el prefijo `pact_pat_` para evitar confundirlos con invitaciones;
- viajan solo en el header `Authorization: Bearer` sobre HTTPS;
- se almacenan en PostgreSQL únicamente como digest;
- tienen vencimiento, `last_used_at` y revocación inmediata;
- no se incluyen en URLs, logs, `pact.yaml` ni `.pact/`.

Los roles de proyecto forman esta jerarquía:

```text
owner > maintainer > contributor > viewer
```

- `viewer`: consulta proyecto, overview y eventos;
- `contributor`: además abre y mantiene sesiones de agentes;
- `maintainer`: además invita contributors y viewers;
- `owner`: además invita maintainers y administra el proyecto.

Solo el bootstrap puede emitir una invitación `owner`. Esta excepción permite
establecer al primer propietario humano; después, el bootstrap debe retirarse
del uso diario. Un owner personal no puede multiplicar owners mediante este
flujo inicial.

Cada sesión de Codex, Claude, Kimi u otro cliente queda patrocinada por el
principal autenticado que la abrió. Otro colaborador no puede enviar
heartbeats ni cerrar esa sesión, salvo un owner o admin de la organización.

## Seguridad y evolución

Este mecanismo no pretende convertirse en un proveedor de identidad. El
secreto de invitación transferido por un canal privado actúa como prueba de
posesión; el correo asociado es identidad declarada, no correo verificado por
PACT.

La siguiente evolución reemplazará la emisión local por OIDC Authorization
Code con PKCE para navegadores y por OAuth Device Authorization Grant para el
CLI. Las membresías y la autorización permanecerán iguales. También se añadirá
almacenamiento nativo en Keychain, Credential Manager o Secret Service; hasta
entonces el CLI protege su configuración global con permisos `0600`.

El diseño sigue las restricciones de privilegio, TLS y no exposición de tokens
descritas en RFC 9700 y RFC 6750. No se implementa el password grant ni se
transportan credenciales en parámetros de URL.

## Consecuencias

- PACT atribuye personas y agentes sin conservar conversaciones;
- revocar un token no afecta a otros computadores del mismo usuario;
- `pact connect` solo enumera proyectos visibles para la identidad;
- un token filtrado tiene alcance y vida limitados, aunque continúa siendo una
  credencial Bearer hasta incorporar DPoP o mTLS;
- el bootstrap deja de ser una credencial compartible y pasa a ser un secreto
  operativo de recuperación.
