# ADR-0001 — Primer recorrido vertical de Pact

**Estado:** propuesto  
**Fecha:** 25 de julio de 2026  
**Decisión que valida:** cuál es el primer escenario que debe demostrar el valor diferencial de Pact.

## Contexto

La visión completa de Pact incluye coordinación multiagente, conocimiento, Git, infraestructura, accesos, secretos, políticas y ejecución. Todas esas capacidades dependen de una hipótesis más pequeña:

> Dos agentes que trabajan sobre el mismo proyecto pueden compartir un estado útil, coordinar sus cambios y transferir contexto sin leer las conversaciones del otro.

Si Pact no demuestra esta hipótesis, añadir infraestructura, pgvector, reuniones o una IA orquestadora aumentaría la complejidad sin probar el fundamento.

## Decisión propuesta

El primer usuario será:

- una persona desarrolladora;
- una sola máquina;
- un repositorio Git;
- dos o más agentes o chats;
- trabajo local;
- sin dependencia de un servicio externo.

La instalación inicial tendrá:

```text
Agente A ─┐
Agente B ─┼── API local ── Pact Server ── PostgreSQL + pgvector
Agente C ─┘                     │
                                └── Pact Node ── Git + worktrees
```

Pact Server y PostgreSQL se iniciarán mediante Docker Compose. Pact Node se ejecutará en el host para observar Git y administrar worktrees sin montar todo el sistema de archivos dentro del servidor.

## Bucle que debe funcionar

```text
1. Un agente abre una sesión.
2. Declara qué intenta conseguir.
3. Pact registra la revisión Git base.
4. Pact entrega el contexto estructurado disponible.
5. El agente declara qué recursos cree que afectará.
6. Pact crea un worktree aislado.
7. Un segundo agente repite el proceso.
8. Pact detecta scopes solapados.
9. Ambos reciben una actualización relevante.
10. Cada agente modifica únicamente su workspace.
11. Pact observa los diffs.
12. Un agente presenta un ChangeSet inmutable.
13. Pact ejecuta validaciones básicas.
14. Pact integra o explica por qué no puede integrar.
15. El proyecto y los agentes reciben la nueva revisión.
```

## Alcance incluido

- proyecto local;
- un repositorio Git;
- actores y sesiones;
- intenciones;
- scopes declarados y observados;
- presencia mediante heartbeat;
- eventos duraderos;
- suscripción a actualizaciones;
- Pact Node;
- worktrees;
- ChangeSets;
- validación básica;
- integración local;
- cambios Git externos;
- contexto estructurado sin LLM;
- persistencia y recuperación después de reiniciar.

## Alcance deliberadamente posterior

Estas capacidades forman parte de Pact, pero no son necesarias para comprobar el primer bucle:

- reuniones y transcripciones;
- conectores documentales;
- embeddings de todo el proyecto;
- síntesis mediante IA;
- conflictos semánticos avanzados;
- Terraform y OpenTofu;
- runners remotos;
- gestores de secretos;
- OIDC;
- múltiples organizaciones;
- Git remoto protegido;
- infraestructura productiva;
- UI web completa.

Las interfaces deben permitir incorporarlas, pero el primer recorrido no simulará que ya existen.

## Modo de gobierno

El comportamiento inicial combinará:

- **Managed para agentes:** Pact crea sus workspaces y ChangeSets.
- **Observer para cambios externos:** una persona puede utilizar Git directamente y Pact reconcilia lo ocurrido.

Pact no impedirá comandos Git locales. Todavía no controlará una rama remota protegida.

## Contexto inicial

El primer `ContextPacket` no utilizará vectores ni un LLM. Contendrá:

- proyecto;
- revisión canónica;
- intención;
- agentes e intenciones activas;
- scopes declarados y observados;
- cambios recientes;
- conflictos conocidos;
- eventos posteriores a un cursor;
- advertencias de frescura.

Esto permite validar la semántica del contexto antes de optimizar su recuperación.

## Criterio de éxito

El recorrido se considera demostrado cuando:

1. Dos agentes crean sesiones independientes bajo la misma persona.
2. Cada uno obtiene su propio worktree desde el mismo commit.
3. Declaran intenciones diferentes.
4. Sus scopes se solapan.
5. Pact avisa a ambos sin bloquear automáticamente su trabajo.
6. Cada uno puede consultar un estado compartido actualizado.
7. Uno entrega un ChangeSet.
8. La validación queda vinculada al hash exacto.
9. La integración actualiza la revisión canónica.
10. El segundo agente recibe que su base quedó obsoleta.
11. Reiniciar Pact no pierde sesiones cerradas, intenciones, eventos ni ChangeSets.
12. Un cambio Git directo se detecta como externo y no se atribuye falsamente a un agente.

## Métrica principal

> Tiempo y cantidad de intervención manual necesarios para que el segundo agente comprenda un cambio relevante realizado por el primero.

La primera demostración debe conseguirlo sin copiar conversaciones ni pedir al usuario que redacte un documento manual.

## Consecuencias

### Positivas

- prueba el núcleo diferencial;
- utiliza la misma PostgreSQL prevista para equipos;
- produce un recorrido observable de extremo a extremo;
- obliga a resolver identidad, revisión, eventos y concurrencia;
- evita depender inicialmente de calidad probabilística.

### Costes

- Docker y PostgreSQL hacen más pesada la instalación personal;
- Pact Node y Pact Server deben coordinarse desde el comienzo;
- el contexto inicial será menos inteligente que la visión completa;
- la integración local todavía no ofrece garantías de un servidor Git protegido.

## Decisiones posteriores relacionadas

- identidad y path del módulo Go;
- licencia;
- sistema operativo soportado primero;
- transporte público inicial;
- forma exacta de integrar cambios locales;
- primer adaptador para chats/agentes;
- política de limpieza de worktrees;
- conjunto de validaciones predeterminado.

