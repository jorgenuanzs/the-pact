# ADR-0007 — Pact Node y observación Git privada

**Estado:** aceptado
**Fecha:** 13 de agosto de 2026

## Contexto

Pact Server no tiene acceso al checkout local y una sesión activa no demuestra
que el código esté cambiando. El backoffice ya distinguía `unobserved`, `idle`,
`editing` y `recent`, pero faltaba el proceso capaz de aportar evidencia.

Enviar un diff completo sería innecesario para este indicador, costoso y
arriesgado: podría sacar código o nombres sensibles del computador.

## Decisión

El CLI incorpora dos modos de observación:

```text
pact node run    observa cambios humanos, del IDE y de herramientas locales
pact agent run   observa mientras vive el proceso del agente envuelto
```

Ambos crean una sesión patrocinada por la identidad personal autenticada,
anuncian `workspace.diff.observe.v1`, mantienen heartbeat y consultan el estado
Git local. Pact Node usa dos segundos como intervalo predeterminado y permite
entre 250 milisegundos y un minuto.

Cada snapshot contiene:

- dirty/clean;
- cantidad de rutas modificadas;
- HEAD y rama, cuando existen;
- una huella SHA-256 del estado porcelain de Git y metadatos locales de las
  rutas modificadas.

Los nombres de rutas forman parte únicamente del cálculo local de la huella.
No se incluyen en el comando HTTP, eventos, logs ni PostgreSQL. Tampoco se leen
ni transmiten contenidos de archivos.

El endpoint de dominio es:

```text
POST /v1/agent-sessions/{session_id}/repository-observations
```

Requiere `Idempotency-Key`. Solo el principal que patrocina la sesión puede
utilizarlo, y la sesión debe estar activa y haber anunciado la capacidad de
observación. El servidor asigna los tiempos y la atribución; no los acepta del
payload.

PostgreSQL conserva el snapshot actual en
`coordination.repository_observations`. Si aparece o cambia una huella dirty,
el comando emite `pact.workspace.diff_updated.v1`. Si HEAD cambia sin un nuevo
diff, emite `pact.git.external_change_detected.v1`. Snapshot, evento, outbox y
resultado idempotente se confirman en una transacción.

No se genera un evento por heartbeat ni por cada polling sin cambios. Esto
evita convertir el log durable en telemetría repetitiva.

## Límites conscientes

- La huella usa metadatos del archivo para detectar nuevas escrituras; no es un
  hash criptográfico del contenido del repositorio ni pretende verificarlo.
- `editing` significa que cambió la evidencia en la ventana activa, no que PACT
  observe pulsaciones de teclado.
- Pact Node todavía se ejecuta en primer plano; la instalación como servicio
  nativo de cada sistema operativo queda para una iteración posterior.
- Este corte observa el checkout actual, pero todavía no crea ni administra
  worktrees aislados.

## Consecuencias

- el backoffice puede distinguir falta de observación, reposo y cambios reales;
- la atribución procede de la sesión autenticada;
- la privacidad no depende de filtrar rutas después de recibirlas: nunca salen
  del host;
- los eventos pueden recuperarse por cursor y sobreviven a la desconexión;
- un agente que no use el wrapper necesita Pact Node activo para ser observado.
