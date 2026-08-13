# ADR-0003 — Semántica de actividad de código observada

**Estado:** aceptado
**Fecha:** 25 de julio de 2026
**Decisión que implementa:** cómo debe interpretar y presentar Pact la
actividad de código en el backoffice.

## Contexto

El backoffice necesita responder dos preguntas diferentes:

- qué trabajo, sesiones, intenciones, workspaces y eventos conoce Pact;
- si existe evidencia de que el código está cambiando en este momento o cambió
  recientemente.

La primera pregunta puede contestarse desde el estado de Pact Server. La
segunda requiere observación del repositorio. Pact Server no monta el sistema
de archivos del usuario ni ejecuta Git, por lo que solo Pact Node podrá producir
esa telemetría.

Una sesión activa indica presencia. Un workspace activo indica que existe un
espacio de trabajo asignado. Ninguno de los dos hechos demuestra por sí solo
que una persona o agente esté modificando código. Presentarlos como edición
sería una inferencia falsa.

## Decisión

Pact separará presencia, trabajo coordinado y actividad de código observada.
El indicador de actividad utilizará cuatro estados:

```text
unobserved
idle
editing
recent
```

Sus significados son:

- `unobserved`: no existe un observador de código vigente ni evidencia reciente.
  Pact desconoce el estado del repositorio. No significa que el código esté
  inactivo.
- `idle`: existe al menos un observador de código vigente, pero no ha informado
  cambios dentro de la ventana reciente.
- `editing`: un observador vigente informó un diff de workspace o un cambio Git
  externo dentro de la ventana activa.
- `recent`: existe evidencia durable de un cambio dentro de la ventana reciente,
  pero Pact no puede afirmar que continúe en curso. La evidencia puede sobrevivir
  a la desconexión del observador.

`editing` significa «Pact detectó una modificación hace pocos segundos». No
significa que pueda observar pulsaciones de teclado, actividad del editor ni
que garantice que el actor siga escribiendo en ese instante.

### Evidencia autorizada

La actividad de código solo se deriva de telemetría procesada mediante comandos
de dominio de Pact Node y de los eventos duraderos emitidos por Pact Server:

```text
pact.workspace.diff_updated.v1
pact.git.external_change_detected.v1
pact.changeset.created.v1
```

Un `ChangeSet` reciente aporta evidencia de cambio, pero nunca produce por sí
solo el estado `editing`: representa contenido ya capturado. Un cambio Git
externo puede demostrar actividad sin atribuirla a un agente.

Los tiempos operativos se calculan con `recorded_at`, asignado por Pact Server,
para no depender del reloj del cliente. Las atribuciones de actor y sesión
proceden de la autenticación del comando; no se aceptan ciegamente desde un
payload de telemetría.

No se utilizarán como prueba de edición:

- `session.status = active`;
- `workspace.status = active`;
- `node.lifecycle_status = active`;
- cambios en `updated_at` de entidades de coordinación;
- la mera existencia de una intención o un scope.

Estos datos se muestran por separado como presencia y trabajo activo.

### Observador vigente

Un observador cuenta como conectado solo cuando:

- su sesión está `active` y no ha expirado;
- la sesión informó un heartbeat durante los últimos 30 segundos;
- la sesión está vinculada a un nodo `active`;
- el nodo informó un heartbeat durante los últimos 30 segundos;
- la sesión anunció la capacidad
  `{"workspace.diff.observe.v1": true}`.

Una sesión sin esa capacidad puede participar en el proyecto, pero no permite
concluir si el repositorio está siendo observado.

### Ventanas

El primer perfil utiliza:

```text
frescura del observador: 30 segundos
ventana editing:          15 segundos
ventana recent:           15 minutos
```

La evaluación sigue este orden:

1. Si hay un observador vigente y el último evento relevante es un diff o
   cambio Git externo dentro de 15 segundos, el estado es `editing`.
2. Si el último evento relevante ocurrió dentro de 15 minutos, el estado es
   `recent`.
3. Si hay un observador vigente y no hay evidencia reciente, el estado es
   `idle`.
4. En cualquier otro caso, el estado es `unobserved`.

Las ventanas forman parte de la respuesta del overview para que la interfaz no
oculte la semántica utilizada. Podrán convertirse en política configurable sin
reinterpretar eventos históricos.

### Actualización del backoffice

El backoffice combina dos mecanismos:

- SSE recuperable por cursor para eventos duraderos;
- consulta periódica de
  `GET /v1/projects/{project_id}/overview` para presencia y heartbeats.

Los heartbeats actualizan `last_seen_at`; no generan un evento durable en cada
intervalo. Por ello, SSE por sí solo no puede detectar que un observador acaba
de quedar obsoleto. El cliente refresca el overview aproximadamente cada cinco
segundos y también lo actualiza al recibir eventos relevantes.

La carga inicial obtiene un snapshot y el stream permite continuar desde el
último cursor conocido, evitando depender de mensajes efímeros como fuente de
verdad.

### Entrada de telemetría

Pact no ofrecerá un endpoint genérico `publish_event`. Un actor autorizado
envía comandos de dominio como observar un diff, registrar un cambio Git externo
o crear un ChangeSet. Pact valida identidad, sesión, proyecto, versiones e
idempotencia y después emite el evento correspondiente.

Esto impide que un agente fabrique directamente la historia de auditoría o
eluda las invariantes del dominio.

## Estado de implementación

El backoffice, Pact Node y el wrapper `pact agent run` implementan esta
semántica. El servidor conserva la última observación por sesión y emite eventos
duraderos cuando cambia un diff observado o HEAD. Si no existe un observador
compatible, el resultado correcto sigue siendo `unobserved`, aunque haya otras
sesiones o workspaces registrados.

## Consecuencias

### Positivas

- el panel distingue hechos observados de inferencias;
- una desconexión no borra evidencia reciente;
- la presencia no se confunde con edición;
- los eventos siguen siendo recuperables y auditables;
- la futura incorporación de Pact Node no exige cambiar el significado de los
  estados actuales.

### Costes

- el backoffice necesita polling además de SSE;
- `editing` es una aproximación temporal, no presencia dentro de un editor;
- sin Pact Node, el indicador permanece deliberadamente limitado;
- las consultas agregadas deberán convertirse en proyecciones si el volumen
  hace costoso calcularlas en cada refresco.
