# ADR-0017 — Cuentas locales, sesiones web y autorización de dispositivos

**Estado:** aceptado  
**Fecha:** 17 de agosto de 2026  
**Reemplaza:** ADR-0006 para autenticación; conserva su modelo de membresías y roles

## Contexto

La primera vertical de Pact utilizó una credencial bootstrap y tokens personales
Bearer de treinta días. Permitió validar permisos e invitaciones, pero convirtió
un secreto copiable en la identidad efectiva de una persona y obligó a pegar el
token en el CLI y el backoffice.

Pact todavía está en alpha. No existe una obligación de compatibilidad que
justifique consolidar esa solución provisional.

## Decisión

Pact tendrá autenticación local convencional e invite-only:

- el primer owner se registra con correo, usuario y contraseña mediante un
  código de setup de un solo uso;
- el resto de las cuentas nace al aceptar una invitación;
- las contraseñas se almacenan únicamente como hashes Argon2id;
- el navegador usa una sesión opaca en una cookie `HttpOnly`, `Secure` cuando
  hay HTTPS y `SameSite=Strict`;
- las mutaciones hechas con una sesión web requieren un token CSRF separado;
- el CLI inicia un Device Authorization Flow y nunca recibe la contraseña;
- el dispositivo obtiene una credencial revocable sin que la persona tenga que
  copiarla o verla;
- las sesiones de agentes continúan patrocinadas por el principal que autorizó
  el dispositivo;
- los tokens `pact_pat_*` y `PACT_LOCAL_API_TOKEN` dejan de ser aceptados.

El código de setup se configura temporalmente mediante `PACT_SETUP_TOKEN`. Una
vez que existe el primer owner local, el endpoint de setup queda cerrado aunque
la variable continúe presente. Se recomienda eliminarla y reiniciar el servicio.

## Flujo web

```text
correo o usuario + contraseña
              ↓
      sesión web opaca
              ↓
 cookie HttpOnly + prueba CSRF
```

No habrá registro público en la primera versión. Una invitación es el derecho a
crear una cuenta dentro de la organización y proyecto indicados.

## Flujo CLI

```text
pact login
    ↓
device_code + user_code
    ↓
el navegador abre Pact y el usuario inicia sesión
    ↓
el usuario aprueba el computador
    ↓
el CLI intercambia device_code por una credencial de dispositivo
```

La credencial de dispositivo sigue siendo un secreto técnico. La diferencia es
que es específica del computador, revocable, expira y no es una identidad
humana ni se transfiere manualmente a un agente. El almacenamiento en Keychain,
Credential Manager y Secret Service sustituirá el archivo protegido `0600` sin
cambiar el protocolo.

## Seguridad

- Los errores de login no revelan si el correo o usuario existe.
- Los intentos fallidos producen bloqueo temporal.
- Cambiar la contraseña revoca las demás sesiones web y todos los dispositivos.
- Las invitaciones y códigos de dispositivo son de un uso y se guardan como
  digests SHA-256.
- Las respuestas de autenticación usan `Cache-Control: no-store`.
- Ninguna contraseña, cookie o credencial de dispositivo entra en `pact.yaml`,
  `.pact/`, eventos, prompts o logs.

## Evolución

GitHub, Google, Microsoft, OIDC, SAML y passkeys serán nuevos autenticadores
vinculados al mismo principal. No cambiarán organizaciones, membresías, roles,
agentes, intenciones ni auditoría.
