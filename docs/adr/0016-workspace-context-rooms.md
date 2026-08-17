# ADR-0016 — Rooms humanas de contexto con menciones explícitas

**Estado:** aceptado

**Fecha:** 17 de agosto de 2026

## Contexto

PACT ya distingue presencia, intención coordinada, scopes, worktrees,
conocimiento revisado y Context Packs. Convertir cada prompt o modificación
pequeña en un intent o una room produciría demasiada metadata, multiplicaría
espacios efímeros y consumiría contexto del modelo sin aportar coordinación
real.

Sin embargo, una parte importante de un proyecto no nace como código ni como
un registro formal: conversaciones con clientes, observaciones de producto,
resúmenes de reuniones y preguntas abiertas necesitan un lugar compartido en
el que personas y agentes puedan participar con lenguaje natural.

## Decisión

Cada Workspace contiene un conjunto pequeño de rooms durables creadas
deliberadamente por personas. PACT crea solamente `#general` de forma
automática al crear el Workspace. Las demás rooms requieren una acción
explícita de un maintainer.

Una room:

- pertenece al Workspace, no a una sesión, prompt, intent, rama o repositorio;
- tiene nombre, slug, descripción y ciclo de vida propios;
- conserva mensajes atribuidos a actores humanos o a sesiones de agentes;
- permite respuestas y threads mediante referencias entre mensajes;
- admite menciones explícitas a actores elegibles del Workspace;
- no crea intents, scopes, ramas, worktrees ni registros de conocimiento;
- no se incorpora automáticamente al contexto de ningún agente.

Las menciones se materializan como items de inbox con estados `pending`,
`read`, `responded` y `dismissed`. Mencionar a un agente no lo ejecuta ni lo
despierta: su cliente consulta el inbox mediante MCP y decide cuándo leer una
ventana acotada de la room. Al responder puede confirmar la mención como
atendida.

## Frontera de contexto

El adaptador MCP expone una sola herramienta compacta, `pact.rooms`, con
acciones para listar rooms y participantes, leer mensajes, publicar,
consultar menciones y reconocerlas. El historial se obtiene solamente al
invocar la acción `read`, con un máximo de cien mensajes y cuarenta por
defecto. También puede limitarse por thread, cursor anterior o búsqueda de
texto completo.

Las instrucciones MCP declaran expresamente que una room es contexto soft
organizado por humanos. El agente debe consultarla cuando una persona se lo
pida o cuando atienda una mención; no debe recorrer todas las rooms al iniciar
cada tarea.

## Persistencia e interfaz

PostgreSQL mantiene rooms, mensajes y menciones en el esquema
`collaboration`. Los cuerpos se indexan con búsqueda de texto completo simple,
sin depender de embeddings ni de un modelo de IA.

Pact Control muestra las rooms en el resumen del Workspace, actualiza la room
seleccionada en intervalos breves y ofrece composición con `@`, respuestas e
inbox personal. La UI solo solicita los últimos mensajes de la room abierta.

## Relación con el conocimiento durable

El chat no sustituye `knowledge.records`. Una conversación puede contener una
decisión candidata, pero continúa siendo contexto informal hasta que una
persona o agente propone un registro tipado y este pasa por su ciclo de
revisión. Esta separación evita que cada frase se convierta en verdad oficial
del proyecto.

## Consecuencias

- las personas controlan la topología conversacional y evitan miles de rooms;
- humanos y agentes comparten contexto natural sin convertir cada interacción
  en trabajo coordinado;
- el costo de tokens permanece acotado y bajo demanda;
- las menciones son señales durables, no un sistema de ejecución remota;
- PACT no intenta reemplazar Slack ni capturar conversaciones privadas de los
  clientes de IA;
- una futura capa de notificaciones o runners podrá reaccionar a menciones sin
  cambiar el modelo persistente, pero requerirá una decisión independiente.
