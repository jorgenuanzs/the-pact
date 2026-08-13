# Pact — Documento maestro de producto, arquitectura y construcción

**Estado:** visión integral inicial  
**Versión:** 0.1  
**Fecha:** 25 de julio de 2026  
**Propósito:** definir qué debe ser Pact, qué debe construir, cómo deben relacionarse sus piezas y qué condiciones debe cumplir para convertirse en una plataforma segura y útil.

**Contenido**

1. Resumen ejecutivo
2. Definición del producto
3. Usuarios y escenarios
4. Principios e invariantes
5. Vocabulario de dominio
6. Arquitectura objetivo
7. Topologías de despliegue
8. Protocolo Pact
9. Coordinación de trabajo
10. Integración con Git
11. Arquitectura de datos
12. Memoria y conocimiento
13. Ingestión de fuentes
14. Búsqueda vectorial y recuperación híbrida
15. Compilador de contexto
16. Papel de la inteligencia artificial
17. Identidad, delegación y autorización
18. Secretos y credenciales
19. Infraestructura y ejecución
20. Auditoría, observabilidad y evidencia
21. Experiencia de usuario y herramientas
22. Stack tecnológico objetivo
23. Seguridad y modelo de amenazas
24. Requisitos operativos
25. Mapa completo de construcción
26. Dependencias y orden de construcción
27. Estrategia de pruebas
28. Escenarios de aceptación
29. Métricas
30. Riesgos
31. Decisiones adoptadas
32. Decisiones abiertas
33. Ejemplo de configuración
34. Criterio de producto completo
35. Referencias técnicas

---

## 1. Resumen ejecutivo

Pact es un plano de control para proyectos en los que personas y agentes de IA comparten conocimiento, coordinan trabajo y realizan acciones verificables sobre código, infraestructura y otros recursos.

Git conserva los archivos y su historia. Las herramientas de infraestructura administran recursos. Los gestores de secretos protegen credenciales. Los sistemas documentales conservan material original. Pact no debe reemplazar indiscriminadamente estos sistemas: debe conectarlos mediante un modelo común de identidad, intención, capacidades, contexto, eventos y evidencia.

Pact debe responder preguntas que hoy quedan repartidas entre chats, repositorios, herramientas de gestión, documentación y conocimiento tácito:

- ¿Cuál es el estado actual del proyecto?
- ¿Qué personas y agentes están trabajando ahora?
- ¿Qué intenta conseguir cada participante?
- ¿Sobre qué revisión y qué componentes está trabajando?
- ¿Qué decisiones, restricciones y requisitos son relevantes?
- ¿Qué cambios pueden interferir entre sí aunque no toquen el mismo archivo?
- ¿Qué puede hacer cada humano o agente y durante cuánto tiempo?
- ¿Qué infraestructura existe en cada ambiente?
- ¿Qué acción fue ejecutada, por quién, bajo qué delegación y con qué resultado?
- ¿Qué conocimiento sigue vigente y cuál quedó obsoleto?

La tesis central es:

> El contexto de un proyecto no debe ser una conversación enorme ni una colección de archivos que alguien debe mantener manualmente. Debe ser un estado vivo, estructurado, consultable, versionado y respaldado por evidencias.

La visión de largo plazo es que Pact funcione como el **runtime del proyecto**:

```text
Git conserva el pasado del artefacto.
Pact representa el presente del trabajo.
Pact ayuda a evaluar y ejecutar el siguiente cambio seguro.
```

Este documento describe la visión completa. No limita el producto a un MVP. El orden de construcción aparece al final como una estrategia para llegar a la arquitectura completa sin perder coherencia.

---

## 2. Definición del producto

### 2.1 Definición corta

> Pact es un protocolo y una plataforma de coordinación, conocimiento y acceso para proyectos operados conjuntamente por humanos y agentes de IA.

### 2.2 Definición operativa

Pact mantiene un modelo vivo de:

- organizaciones y proyectos;
- humanos, agentes, nodos y ejecutores;
- sesiones, intenciones, tareas y reservas temporales;
- repositorios, revisiones, artefactos y relaciones;
- decisiones, requisitos, restricciones, hechos e incertidumbres;
- ambientes, infraestructura, servicios y dependencias;
- capacidades delegadas, políticas y aprobaciones;
- eventos, acciones, resultados y evidencias;
- fuentes documentales, fragmentos, índices y paquetes de contexto.

Pact utiliza ese modelo para:

- coordinar trabajo concurrente;
- generar contexto específico para cada intención;
- anticipar conflictos;
- controlar acciones sensibles;
- conservar memoria institucional;
- relacionar código, infraestructura y decisiones;
- permitir auditoría y reconstrucción causal;
- reducir la necesidad de transferir manualmente archivos, claves y explicaciones entre chats.

### 2.3 Qué no debe ser

Pact no debe convertirse en:

- un reemplazo de Git;
- un editor colaborativo de texto;
- un chat gigante utilizado como memoria;
- un gestor de contraseñas construido desde cero;
- una alternativa propietaria a Terraform, OpenTofu o Kubernetes;
- una única IA omnisciente que toma todas las decisiones;
- un índice vectorial presentado como fuente de verdad;
- una excusa para entregar acceso irrestricto a agentes;
- una copia descontrolada de todo el contenido de la organización;
- un sistema que solo funciona si nadie lo evita o comete errores.

### 2.4 Sistemas de autoridad

Cada tipo de información debe tener una única autoridad editable.

| Dominio | Autoridad primaria | Papel de Pact |
|---|---|---|
| Código y archivos | Git | Observar, relacionar, coordinar e integrar |
| Historia de cambios | Git + registro de Pact | Añadir intención, causalidad y evidencia |
| Estado operativo de Pact | PostgreSQL | Autoridad |
| Eventos de Pact | Registro inmutable en PostgreSQL | Autoridad |
| Infraestructura declarada | Git + IaC | Indexar, planificar y gobernar |
| Estado real de infraestructura | APIs del proveedor + estado IaC | Observar y reconciliar |
| Secretos | Gestor de secretos o identidad de carga | Referenciar y mediar, nunca exponer |
| Documentos originales | Sistema de origen u object storage | Ingerir, clasificar e indexar |
| Embeddings y resúmenes | Índices derivados | Regenerables, nunca autoridad |
| Presencia y reservas | Pact | Autoridad temporal |
| Políticas de proyecto | Archivos versionados + política compilada | Evaluar y auditar |

La regla es:

> Pact debe saber dónde vive la verdad, qué significa, quién puede usarla y qué acciones están permitidas.

---

## 3. Usuarios y escenarios

### 3.1 Desarrollador individual con varios agentes

Una persona ejecuta varios chats o agentes en la misma máquina. Todos se conectan al mismo proyecto de Pact, reciben identidades de sesión diferentes y trabajan en espacios aislados. Pact registra intenciones, detecta solapamientos y evita que cada agente dependa de la transcripción de los demás.

### 3.2 Equipo humano con agentes personales

Cuatro personas trabajan simultáneamente, cada una con uno o más agentes. Los nodos locales observan el trabajo de su máquina; un Pact Server compartido mantiene el estado común. La identidad del agente siempre conserva la cadena de delegación hacia la persona responsable.

### 3.3 Operación de infraestructura

Un agente necesita investigar un incidente en `staging`. Pact le entrega topología, documentación y una capacidad temporal de solo lectura. Si propone reiniciar un servicio, la política evalúa el riesgo, solicita aprobación si corresponde y un runner autorizado ejecuta la operación sin revelar las credenciales al agente.

### 3.4 Cambio de código con impacto semántico

Un agente modifica el contrato `Session`; otro crea un endpoint que lo consume en un archivo diferente. Git no encuentra un conflicto textual. Pact utiliza intenciones, grafo de símbolos y contratos para advertir un conflicto semántico antes de la integración.

### 3.5 Reunión con un cliente

Una transcripción se incorpora como fuente restringida. Pact extrae solicitudes, decisiones, compromisos, actores y preguntas abiertas, siempre conservando la procedencia. Meses después, un agente puede explicar por qué existe una restricción enlazando la reunión, la decisión técnica, el código y la prueba correspondiente.

### 3.6 Incorporación de una persona o un agente

En lugar de leer todas las conversaciones históricas, el nuevo participante solicita un contexto para una intención concreta. Pact genera una vista con objetivos, arquitectura, decisiones vigentes, trabajos activos, restricciones, recursos y fuentes relevantes.

### 3.7 Cambio externo a Pact

Una persona realiza un cambio directamente en Git. Pact lo detecta, lo registra como externo, actualiza índices, invalida conocimiento obsoleto y avisa a trabajos afectados. El sistema no depende de un comportamiento perfecto de todos los participantes.

---

## 4. Principios e invariantes

Estas reglas deben considerarse parte de la constitución del producto.

### 4.1 Git sigue siendo la autoridad sobre los archivos

Pact puede crear worktrees, ramas, parches y propuestas de integración, pero no debe inventar un formato propietario de repositorio para el código.

### 4.2 El proyecto vivo no equivale a un directorio mutable compartido

El estado de coordinación puede actualizarse continuamente. Los cambios de código deben producirse en espacios aislados y llegar a la versión canónica mediante integraciones atómicas y verificadas.

### 4.3 Toda acción tiene identidad, intención y revisión

No debe existir una acción relevante sin:

- actor;
- responsable o cadena de delegación;
- intención;
- proyecto y ambiente;
- revisión base;
- instante;
- resultado;
- evidencia.

### 4.4 Toda afirmación importante tiene procedencia y vigencia

Un hecho debe indicar:

- de dónde proviene;
- quién o qué lo generó;
- cuándo fue observado;
- para qué revisión o periodo es válido;
- si fue declarado, observado, inferido o verificado;
- su nivel de confianza;
- qué evidencia permite comprobarlo.

### 4.5 El contexto se compila, no se acumula

Pact no entrega toda la memoria disponible. Genera un paquete acotado según intención, identidad, permisos, revisión y presupuesto de contexto.

### 4.6 Los permisos se aplican antes de recuperar contexto

Una búsqueda semántica no debe descubrir contenido que el actor no puede consultar. El filtrado de autorización ocurre antes de búsqueda textual, vectorial, expansión de grafo y síntesis.

### 4.7 Las IA proponen; el núcleo determinista decide y registra

Las IA pueden inferir impacto, resumir, planificar o explicar. Las sesiones, permisos, transacciones, reservas, políticas y registros no dependen de una interpretación probabilística.

### 4.8 Entregar capacidades, no secretos

Los agentes deben recibir permiso temporal para ejecutar una operación, no contraseñas permanentes. Los valores secretos no entran en prompts, eventos, trazas ni documentos de contexto.

### 4.9 Denegar por defecto

La ausencia de una política explícita no concede acceso. Las acciones destructivas, los ambientes sensibles y los datos confidenciales requieren autorizaciones más fuertes.

### 4.10 Permisivo al crear, estricto al integrar

Pact permite explorar y producir cambios en espacios aislados. La incorporación a estados canónicos se somete a compatibilidad, validaciones, políticas y aprobaciones.

### 4.11 Los índices derivados deben poder reconstruirse

Embeddings, resúmenes, grafos inferidos y cachés se pueden eliminar y regenerar desde fuentes autorizadas y eventos duraderos.

### 4.12 Las conversaciones no son conocimiento validado

Una conversación puede contener hipótesis, contradicciones y decisiones descartadas. Pact conserva hechos, decisiones, requisitos, preguntas y resultados estructurados; la transcripción original permanece como evidencia con su clasificación correspondiente.

### 4.13 El protocolo es independiente del proveedor

Pact debe permitir diferentes modelos de IA, proveedores Git, gestores de secretos, herramientas IaC, nubes y sistemas documentales.

### 4.14 El sistema debe reconciliarse con la realidad

Pact no puede asumir que su base de datos siempre refleja Git, la infraestructura o los sistemas externos. Debe observar, comparar, detectar deriva y reparar su representación.

---

## 5. Vocabulario de dominio

| Término | Definición |
|---|---|
| Organización | Límite administrativo y de confianza que contiene proyectos |
| Proyecto | Unidad de coordinación, conocimiento y políticas |
| Humano | Persona autenticada responsable de acciones propias o delegadas |
| Agente | Instancia de software o IA con identidad, sesión y capacidades limitadas |
| Nodo | Proceso local asociado a una máquina o entorno que observa y ejecuta operaciones locales |
| Runner | Ejecutor aislado autorizado para realizar trabajos y acceder temporalmente a recursos |
| Sesión | Presencia temporal de un humano, agente, nodo o runner |
| Intención | Resultado que un actor intenta conseguir y contexto causal de su trabajo |
| Tarea | Unidad concreta de trabajo dentro de una intención |
| Scope | Conjunto declarado, observado o inferido de recursos afectados |
| Lease | Reserva temporal renovable y con vencimiento |
| Capacidad | Autorización concreta para realizar una acción sobre un recurso bajo condiciones |
| Delegación | Concesión limitada de capacidades de un principal a otro actor |
| Revisión | Identificador de un estado del proyecto, normalmente un commit Git |
| Artefacto | Archivo, símbolo, documento, servicio, esquema, endpoint, plan o resultado |
| Relación | Enlace tipado y respaldado por procedencia entre entidades |
| Hecho | Afirmación estructurada con fuente, validez y confianza |
| Decisión | Elección aceptada, con motivo, alternativas, alcance y estado |
| Restricción | Condición que un cambio o una acción debe respetar |
| Evento | Registro inmutable de algo que ocurrió |
| Comando | Solicitud para producir un cambio de estado |
| Evidencia | Fuente que permite verificar una afirmación, acción o resultado |
| Fuente | Sistema o artefacto original del que se obtuvo información |
| Context packet | Paquete de contexto acotado y trazable para una intención |
| Ambiente | Contexto operativo como local, desarrollo, staging o producción |
| Recurso | Entidad operable: servidor, base de datos, servicio, clúster, secreto o repositorio |
| Política | Regla evaluable que permite, deniega o exige condiciones |
| Aprobación | Consentimiento explícito requerido por una política |

---

## 6. Arquitectura objetivo

### 6.1 Vista general

```text
┌────────────────────────────────────────────────────────────────────┐
│ Personas, chats, agentes, automatizaciones y sistemas externos     │
└──────────────────────────────┬─────────────────────────────────────┘
                               │
             CLI / SDK / API / adaptadores de herramientas
                               │
┌──────────────────────────────▼─────────────────────────────────────┐
│                           PACT SERVER                              │
│                                                                    │
│  Identidad     Coordinación     Eventos       Políticas            │
│  Delegación    Git/workspaces   Conocimiento Aprobaciones          │
│  Contexto      Infraestructura Auditoría      Suscripciones         │
└────────────┬───────────────┬───────────────┬───────────────────────┘
             │               │               │
       PostgreSQL       Object storage   Gestor de secretos
       + pgvector       y artefactos     / KMS / identidad
             │
      outbox y workers
             │
┌────────────▼───────────────────────────────────────────────────────┐
│ Pact Nodes, runners, indexadores y conectores                      │
└───────┬──────────────┬───────────────┬───────────────┬─────────────┘
        │              │               │               │
       Git         Terraform/       Servidores y    Docs, reuniones,
                   OpenTofu/K8s     bases de datos  tickets y fuentes
```

### 6.2 Pact Server

Responsabilidades:

- API pública y control de versiones del protocolo;
- autenticación de humanos, agentes, nodos y runners;
- registro de organizaciones, proyectos y membresías;
- sesiones, presencia y heartbeats;
- intenciones, tareas, scopes y leases;
- registro de comandos y eventos;
- evaluación de políticas;
- gestión de aprobaciones;
- coordinación Git e integración;
- catálogo de artefactos, hechos y relaciones;
- ingestión y búsqueda de conocimiento;
- compilación de contexto;
- orquestación de trabajos duraderos;
- suscripciones en tiempo real;
- auditoría, administración y observabilidad.

El servidor se implementará inicialmente como un monolito modular en Go. Las fronteras internas deben permitir separar workers o servicios solo cuando exista una razón operativa.

### 6.3 Pact Node

Proceso que vive en la máquina de una persona, un host o una red.

Responsabilidades:

- conectar de forma saliente con Pact Server;
- identificar la máquina y los repositorios registrados;
- observar Git y el sistema de archivos;
- crear y administrar worktrees o workspaces;
- ejecutar comandos locales autorizados;
- recopilar resultados y evidencias;
- mantener una cola temporal si se pierde la conexión;
- filtrar secretos antes de enviar logs;
- exponer una interfaz local a chats y agentes;
- aplicar límites de acceso a archivos y procesos.

El nodo no es autoridad global. Publica observaciones y ejecuta capacidades concedidas.

### 6.4 Pact Runner

Ejecutor aislado para:

- pruebas;
- compilaciones;
- análisis estático;
- generación de planes IaC;
- despliegues;
- consultas controladas;
- migraciones;
- operaciones remotas.

Debe soportar:

- entornos efímeros;
- imágenes o toolchains versionadas;
- límites de CPU, memoria, tiempo y red;
- identidad de carga;
- credenciales de corta duración;
- captura de logs y artefactos;
- cancelación;
- idempotencia;
- limpieza verificable.

Puede ejecutarse en la máquina local, CI, contenedores, clústeres o redes privadas.

### 6.5 Workers

Procesos asincrónicos para:

- analizar repositorios;
- extraer símbolos y dependencias;
- procesar documentos;
- transcribir audio;
- dividir contenido;
- generar embeddings;
- extraer hechos y decisiones;
- recalcular relaciones;
- invalidar índices;
- generar resúmenes;
- ejecutar evaluaciones.

Un worker consume trabajos duraderos, informa progreso y produce eventos idempotentes.

### 6.6 Policy engine

La autorización básica puede implementarse en Go, pero las políticas complejas deben expresarse como código versionado y evaluable. Open Policy Agent ofrece una API separada para administrar y evaluar políticas, por lo que es un candidato de integración, no una dependencia semántica obligatoria de Pact. Véase la [API oficial de OPA](https://www.openpolicyagent.org/docs/rest-api).

### 6.7 Context compiler

Servicio lógico que:

1. recibe actor, intención, revisión y presupuesto;
2. calcula el ámbito inicial;
3. aplica permisos;
4. recupera datos estructurados;
5. expande relaciones;
6. realiza búsqueda textual;
7. realiza búsqueda vectorial;
8. combina y reordena resultados;
9. verifica vigencia y procedencia;
10. sintetiza opcionalmente;
11. devuelve un paquete con evidencias y exclusiones.

No debe depender obligatoriamente de una llamada a un LLM. Siempre debe poder entregar contexto estructurado.

### 6.8 Object storage

Los objetos grandes no deben almacenarse indiscriminadamente en PostgreSQL:

- transcripciones originales;
- grabaciones;
- documentos;
- archivos importados;
- logs extensos;
- planes;
- resultados de pruebas;
- builds;
- snapshots;
- parches grandes.

Pact almacenará metadatos, hashes, permisos y referencias en PostgreSQL. La implementación debe usar una interfaz compatible con almacenamiento de objetos para permitir modo local, self-hosted y cloud.

---

## 7. Topologías de despliegue

### 7.1 Personal unificado

El usuario acepta ejecutar PostgreSQL desde el comienzo para mantener la misma semántica que el modo equipo.

```text
Docker Compose o instalación local
├── Pact Server
├── PostgreSQL + pgvector
├── Object storage local opcional
└── Pact Node en el host

Chats y agentes → Pact Node/API local → Pact Server
```

Propiedades:

- un solo usuario;
- varios agentes;
- repositorios locales;
- funcionamiento privado;
- posibilidad de conectores externos;
- copias de seguridad exportables;
- misma API y mismas migraciones que una instalación compartida.

### 7.2 Equipo autohospedado

```text
Pact Server compartido
├── PostgreSQL
├── Object storage
├── proveedor OIDC
├── gestor de secretos
└── workers

Cada persona
└── Pact Node + agentes locales
```

El servidor mantiene el estado común. Los nodos administran archivos, credenciales locales y ejecución en cada máquina.

### 7.3 Servicio administrado

Pact ofrece:

- organizaciones y proyectos;
- base de datos administrada;
- almacenamiento;
- actualizaciones;
- observabilidad;
- copias de seguridad;
- integración con proveedores Git y documentos.

Los clientes mantienen nodos o runners privados para repositorios, redes o secretos que no deben abandonar su infraestructura.

### 7.4 Empresa y redes privadas

Componentes desplegados dentro de la organización:

- Pact Server privado o control plane federado;
- runners por ambiente;
- integración con OIDC corporativo;
- gestor de secretos corporativo;
- claves administradas por el cliente;
- políticas y retención propias;
- auditoría exportable;
- conectividad saliente desde redes restringidas.

### 7.5 Futuro federado

Organizaciones o dominios de confianza diferentes pueden compartir únicamente proyectos, eventos o capacidades explícitas. No se debe asumir confianza global entre instalaciones.

---

## 8. Protocolo Pact

### 8.1 Objetivo

El protocolo permite que humanos, agentes, nodos, runners y conectores cooperen sin depender de una interfaz o proveedor específico.

Debe publicarse mediante:

- especificación OpenAPI para comandos y consultas;
- JSON Schema para recursos y eventos;
- SDK oficial de Go, TypeScript y Python;
- streaming por WebSocket o Server-Sent Events;
- adaptadores para herramientas de agentes;
- webhooks firmados;
- formato de eventos inspirado en CloudEvents.

CloudEvents proporciona un sobre interoperable para identificar eventos y transportar datos entre servicios y plataformas; Pact puede adoptarlo o definir un perfil compatible. Véase la [especificación oficial de CloudEvents](https://github.com/cloudevents/spec).

### 8.2 Separación entre comandos, consultas y eventos

**Comando:** solicita cambiar estado.

```json
{
  "command_id": "cmd_01...",
  "type": "intent.create",
  "project_id": "prj_01...",
  "actor_session_id": "ses_01...",
  "expected_revision": 42,
  "idempotency_key": "agent-a:create:fix-session",
  "data": {}
}
```

**Consulta:** solicita una vista sin cambiar estado.

```json
{
  "project_id": "prj_01...",
  "revision": "git:81af23",
  "intent_id": "int_01...",
  "query": "context"
}
```

**Evento:** registra un hecho ocurrido.

```json
{
  "specversion": "1.0",
  "id": "evt_01...",
  "source": "pact://projects/prj_01",
  "type": "pact.intent.created.v1",
  "time": "2026-07-25T10:30:00Z",
  "subject": "intent/int_01",
  "datacontenttype": "application/json",
  "data": {
    "actor_id": "agt_01...",
    "base_revision": "git:81af23"
  }
}
```

### 8.3 Garantías del protocolo

- Identificadores globalmente únicos.
- Claves de idempotencia obligatorias para comandos mutables.
- Control optimista de concurrencia mediante revisión esperada.
- Orden total por agregado cuando sea necesario, no orden global artificial.
- Eventos inmutables.
- Entrega al menos una vez; consumidores idempotentes.
- Marcas de tiempo del servidor y tiempo declarado por la fuente.
- Correlation ID y causation ID.
- Actor y cadena de delegación.
- Versionado de tipos y payloads.
- Compatibilidad hacia atrás durante una ventana publicada.
- Paginación por cursor.
- Reanudación de streams desde un cursor.
- Respuestas de error estructuradas.

### 8.4 Familias de API

```text
/auth
/organizations
/projects
/memberships
/actors
/agents
/nodes
/runners
/sessions
/delegations
/capabilities
/intents
/tasks
/scopes
/leases
/events
/subscriptions
/repositories
/workspaces
/changes
/integrations
/artifacts
/relations
/facts
/decisions
/constraints
/sources
/documents
/contexts
/environments
/resources
/plans
/executions
/approvals
/policies
/secrets/references
/audit
/admin
```

### 8.5 Presencia y sesiones

Una sesión contiene:

- actor;
- tipo de cliente;
- versión del protocolo;
- capacidades anunciadas;
- proyecto;
- nodo;
- fecha de inicio;
- último heartbeat;
- expiración;
- estado;
- intención activa opcional.

Los heartbeats no se convierten todos en eventos permanentes. Pact conserva transiciones relevantes: conexión, pérdida, expiración, recuperación y cierre.

### 8.6 Suscripciones

Un cliente puede suscribirse a:

- proyecto;
- intención;
- tarea;
- componente;
- recurso;
- ambiente;
- tipo de evento;
- actor;
- cursor de continuación.

PostgreSQL `LISTEN/NOTIFY` puede servir para despertar procesos internos, pero no es el registro durable. La fuente recuperable es la tabla de eventos y su outbox. La [documentación de PostgreSQL](https://www.postgresql.org/docs/current/sql-notify.html) describe `NOTIFY` como un mecanismo sencillo de comunicación entre procesos.

### 8.7 Versionado

- El protocolo tiene versión mayor y menor.
- Cada tipo de evento tiene versión explícita.
- Los campos nuevos son opcionales mientras no exista una versión mayor.
- Los consumidores deben ignorar campos desconocidos.
- Los productores no eliminan ni reinterpretan campos sin migración.
- Las capacidades negociadas permiten activar funciones nuevas.
- Las migraciones de datos y protocolo se prueban contra clientes anteriores.

---

## 9. Coordinación de trabajo

### 9.1 Estado de una intención

```text
draft
  ↓
declared
  ↓
active ─────→ blocked
  │             │
  │             └────→ active
  ↓
submitted
  ↓
validating
  ├────→ changes_requested → active
  ├────→ failed
  └────→ integrated

También:
cancelled
abandoned
superseded
```

### 9.2 Contenido mínimo de una intención

- objetivo;
- explicación;
- actor responsable;
- delegador;
- revisión base;
- criterios de finalización;
- scopes declarados;
- riesgos;
- ambiente;
- capacidades necesarias;
- dependencias;
- presupuesto temporal o económico;
- política de integración;
- visibilidad.

### 9.3 Scope declarado, observado e inferido

Pact distingue:

```text
Declarado: el actor indica qué pretende afectar.
Observado: Pact detecta qué leyó, modificó o ejecutó.
Inferido: analizadores o IA calculan impacto indirecto.
Verificado: una prueba o política demuestra una relación.
```

Nunca se presenta una inferencia como certeza.

### 9.4 Leases

Una lease:

- reserva temporalmente un scope o capacidad;
- tiene propietario;
- incluye TTL;
- puede renovarse;
- expira automáticamente;
- puede ser compartida, exclusiva o simplemente informativa;
- no bloquea para siempre;
- no sustituye la comprobación de compatibilidad.

Scopes posibles:

- repositorio;
- ruta;
- archivo;
- símbolo;
- contrato;
- esquema;
- servicio;
- ambiente;
- recurso de infraestructura;
- decisión o documento.

### 9.5 Detección de conflictos

Niveles:

1. **Textual:** mismas líneas o archivos.
2. **Estructural:** mismos símbolos, módulos o esquemas.
3. **Contractual:** productores y consumidores del mismo contrato.
4. **Operacional:** mismo recurso, ambiente o ventana de despliegue.
5. **Semántico:** objetivos o decisiones incompatibles.
6. **Político:** una combinación viola una regla.

El sistema debe explicar:

- qué solapa;
- qué evidencia lo indica;
- qué nivel de confianza existe;
- qué trabajos están afectados;
- qué opciones de resolución propone.

### 9.6 Comunicación entre agentes

Pact puede ofrecer un canal conversacional, pero el protocolo fundamental usa eventos y objetos estructurados:

```text
intent.created
scope.claimed
finding.published
question.raised
answer.provided
decision.proposed
decision.accepted
constraint.discovered
conflict.detected
change.submitted
validation.completed
work.completed
```

Un mensaje puede contener lenguaje natural, pero las consecuencias duraderas se materializan como hechos, decisiones, restricciones o tareas.

### 9.7 Recuperación ante desaparición de un agente

Al expirar una sesión:

- se dejan de renovar sus leases;
- se revocan capacidades temporales;
- la intención pasa a `orphaned` o `paused`;
- el workspace se conserva durante una política de retención;
- los cambios se guardan como parche o commit temporal;
- se notifica al responsable;
- otro actor puede adoptar la intención conservando la trazabilidad.

---

## 10. Integración con Git

### 10.1 Principio

Pact gobierna el flujo y la integración, no impide que Git siga existiendo como herramienta independiente.

### 10.2 Modos de gobierno

#### Observer

- Git funciona normalmente.
- Pact observa cambios y publica contexto.
- No bloquea commits ni integraciones.
- Adecuado para adopción inicial y proyectos personales.

#### Managed

- Pact crea workspaces para agentes.
- Las ramas no protegidas pueden utilizarse directamente.
- Pact valida integraciones a ramas canónicas.
- Los cambios externos se reconcilian.
- Adecuado para equipos.

#### Strict

- La rama canónica está protegida en el servidor Git.
- Solo una identidad de integración autorizada puede actualizarla.
- Pact exige intención, validaciones y aprobaciones.
- Adecuado para entornos regulados o sensibles.

### 10.3 Workspaces

Git permite múltiples árboles de trabajo vinculados al mismo repositorio. La [documentación de `git worktree`](https://git-scm.com/docs/git-worktree.html) confirma que cada worktree puede tener su propio `HEAD` e índice mientras comparte el repositorio.

Flujo:

1. Resolver revisión base.
2. Crear worktree separado.
3. Asociarlo a sesión e intención.
4. Observar cambios.
5. Ejecutar validaciones en el mismo entorno o uno reproducible.
6. Generar commit o parche.
7. Someter a integración.
8. Retener o limpiar según política.

Las ramas pueden existir como detalle interno. El agente no necesita gestionarlas.

### 10.4 Transacción de integración

```text
1. Congelar propuesta.
2. Verificar identidad e intención.
3. Comparar base con revisión canónica.
4. Recalcular conflictos.
5. Reproducir el cambio sobre la revisión candidata.
6. Ejecutar validaciones obligatorias.
7. Evaluar políticas.
8. Obtener aprobaciones.
9. Crear commit de integración.
10. Actualizar referencia canónica.
11. Registrar evento y evidencias.
12. Reindexar e invalidar contexto.
13. Notificar trabajos afectados.
```

La actualización Git y el registro de Pact no pueden formar una única transacción ACID. Debe utilizarse una saga idempotente con reconciliación:

- preparar;
- integrar;
- observar confirmación del proveedor Git;
- materializar estado;
- compensar o marcar inconsistencia si falla.

### 10.5 Cambios externos

Un webhook o sincronización detecta una revisión desconocida. Pact:

- registra `git.external_change.detected`;
- identifica autor y fuente si es posible;
- calcula diff;
- actualiza artefactos;
- invalida hechos vinculados a revisiones anteriores;
- reevalúa scopes;
- informa a agentes;
- nunca inventa una intención retroactiva como si fuera conocida.

### 10.6 Configuración versionada

Propuesta:

```text
pact.yaml
pact/
  policies/
  environments/
  components/
  context/
  adapters/
.pact/                 # vínculo, caché y runtime local; no versionado
```

No se guardan en la configuración versionada:

- sesiones;
- leases;
- tokens;
- secretos;
- base de datos;
- artefactos temporales;
- conversaciones privadas.

### 10.7 Proyectos con varios repositorios

Una intención puede afectar varios repositorios. Pact debe representar una combinación coherente mediante un `ProjectSnapshot`:

```yaml
projectRevision:
  frontend: commit-a
  backend: commit-b
  infrastructure: commit-c
```

Un `IntegrationBundle` agrupa ChangeSets relacionados. Como Git no ofrece una transacción atómica entre repositorios:

1. se preparan y validan todos los cambios;
2. se fija la revisión candidata de cada repositorio;
3. se actualizan referencias en orden determinista;
4. se confirma cada resultado;
5. solo entonces se publica el nuevo ProjectSnapshot;
6. cualquier estado parcial se muestra de forma explícita;
7. se compensa o revierte únicamente cuando la herramienta lo permita.

Los despliegues deben consumir snapshots aceptados, no una mezcla accidental del último commit de cada repositorio.

---

## 11. Arquitectura de datos

### 11.1 Elección principal

La base canónica de Pact será PostgreSQL con la extensión pgvector.

Razones:

- múltiples clientes y escritores concurrentes;
- transacciones y restricciones fuertes;
- JSONB para extensibilidad controlada;
- búsqueda textual;
- índices variados;
- Row-Level Security como defensa adicional;
- extensiones;
- replicación, recuperación y herramientas operativas;
- posibilidad de conservar estado estructurado, eventos, grafo y vectores en una plataforma inicial unificada.

PostgreSQL utiliza control de concurrencia multiversión para permitir que varias operaciones lean y escriban sin convertir cada acceso en un bloqueo global. Véase la [documentación de concurrencia de PostgreSQL](https://www.postgresql.org/docs/current/mvcc-intro.html).

### 11.2 Modelo event-driven, no event sourcing dogmático

Pact debe conservar eventos inmutables, pero no obligar a reconstruir cada consulta reproduciendo toda la historia.

Cada comando aceptado realiza una transacción:

```text
1. Comprobar identidad, versión esperada e idempotencia.
2. Modificar las tablas de estado.
3. Insertar el evento durable.
4. Insertar el registro outbox.
5. Confirmar la transacción.
6. Publicar asincrónicamente.
```

Las tablas relacionales son proyecciones canónicas y consultables; el event log permite auditoría, sincronización y reconstrucción controlada.

### 11.3 Esquemas lógicos

```text
identity
  organizations
  projects
  principals
  human_identities
  actors
  memberships
  agents
  nodes
  runners
  sessions
  delegations
  capabilities
  revocations

coordination
  intents
  intent_assignments
  intent_dependencies
  tasks
  scopes
  leases
  conflicts
  signals
  handoffs

scm
  repositories
  repository_bindings
  git_refs
  project_snapshots
  workspaces
  changesets
  changeset_versions
  integration_attempts
  external_changes

knowledge
  sources
  artifacts
  artifact_versions
  segments
  entities
  entity_aliases
  knowledge_items
  knowledge_evidence
  relations
  embedding_models
  embeddings
  context_requests
  context_packs
  context_feedback

operations
  environments
  resources
  resource_observations
  action_definitions
  action_requests
  plans
  jobs
  job_attempts
  validations
  policy_bundles
  policy_decisions
  approval_requests
  approvals
  credential_leases

platform
  commands
  events
  outbox
  webhook_deliveries
  idempotency_records
  audit_entries
  evidence_objects
  background_jobs
```

Pueden ser esquemas físicos de PostgreSQL o límites lógicos dentro del código. La separación ayuda a evitar un modelo plano imposible de mantener.

### 11.4 Reglas de modelado

- Identificadores opacos y globalmente únicos.
- `organization_id` y `project_id` explícitos en toda entidad multitenant.
- Relaciones críticas mediante claves foráneas.
- Estados mediante enums o tablas controladas, no strings arbitrarios.
- JSONB para atributos extensibles, no para ocultar el modelo completo.
- Fechas del servidor en UTC.
- Soft delete únicamente cuando retención y auditoría lo requieran.
- Versionado optimista por agregado.
- Restricciones de unicidad para idempotencia.
- Índices definidos a partir de consultas reales.
- Particionamiento solo cuando el volumen lo justifique.
- Datos derivados identificados como tales.
- Ningún valor secreto en campos, eventos o logs.

### 11.5 Multi-tenancy

Capas de defensa:

1. Contexto de organización obligatorio en la API.
2. Autorización en el servicio.
3. Consultas preparadas y repositorios que exigen tenant.
4. Row-Level Security en tablas sensibles.
5. Roles de base de datos sin `BYPASSRLS`.
6. Cifrado y namespaces separados para objetos.
7. Pruebas automáticas de aislamiento.

Cuando se habilita Row-Level Security y no existe una política aplicable, PostgreSQL utiliza denegación por defecto. Debe prestarse atención a propietarios y roles privilegiados. Véase la [documentación oficial de RLS](https://www.postgresql.org/docs/current/ddl-rowsecurity.html).

### 11.6 Registro de eventos

Campos mínimos:

```text
id
organization_id
project_id
project_sequence
event_type
event_version
aggregate_type
aggregate_id
aggregate_version
actor_id
session_id
delegation_id
intent_id
causation_id
correlation_id
occurred_at
recorded_at
git_revision
environment_id
payload
payload_hash
```

El registro es append-only para las identidades normales de la aplicación. Correcciones administrativas se expresan mediante nuevos eventos, no reescritura silenciosa.

### 11.7 Outbox y trabajos duraderos

El outbox evita confirmar un cambio en PostgreSQL y perder su publicación por una caída posterior.

Inicialmente:

- tabla `outbox`;
- workers con `FOR UPDATE SKIP LOCKED`;
- reintentos con backoff;
- dead-letter state;
- idempotencia en consumidores;
- `LISTEN/NOTIFY` solo como señal para reducir latencia.

Si Pact se divide en múltiples servicios o el volumen lo exige, se puede proyectar el outbox hacia un bus durable. PostgreSQL continúa siendo la fuente recuperable de los eventos de dominio.

### 11.8 Backups y recuperación

Requisitos:

- copias completas;
- recuperación a un punto en el tiempo;
- pruebas periódicas de restauración;
- backup de object storage;
- exportación de configuraciones y políticas;
- claves de cifrado respaldadas mediante procedimiento separado;
- RPO y RTO definidos por modalidad;
- herramienta de diagnóstico que compare eventos, proyecciones, Git y recursos externos.

No se considera un backup válido hasta haber probado su restauración.

---

## 12. Memoria y conocimiento del proyecto

### 12.1 Tipos de memoria

Pact debe distinguir cinco memorias:

| Memoria | Contenido | Naturaleza |
|---|---|---|
| Evidencia | Documentos, commits, transcripciones, planes y resultados originales | Inmutable y citable |
| Episódica | Eventos, trabajos, despliegues, incidentes y resultados | Histórica |
| Semántica | Hechos, decisiones, requisitos, restricciones y relaciones | Versionada y gobernada |
| Procedimental | Políticas, runbooks, validaciones y formas autorizadas de actuar | Ejecutable o consultable |
| De trabajo | Sesiones, intenciones, scopes, leases y contexto reciente | Temporal |

Una frase casual de una reunión no puede tener el mismo estatus que una decisión aprobada.

### 12.2 Ontología inicial

Tipos de conocimiento:

- hecho;
- decisión;
- requisito;
- restricción;
- compromiso;
- supuesto;
- riesgo;
- hallazgo;
- pregunta abierta;
- política;
- procedimiento;
- incidente;
- resultado de validación.

Estados:

```text
proposed
verified
accepted
disputed
superseded
revoked
expired
stale
rejected
```

`confidence` mide incertidumbre de extracción. `authority_level` mide autoridad de la fuente o aprobación. No deben confundirse.

### 12.3 Artefactos y versiones

```text
source
  Sistema original: Git, Drive, importación, reunión, ticket, CI, etc.

artifact
  Identidad estable del objeto.

artifact_version
  Snapshot inmutable de una versión.

segment
  Fragmento citable con ancla a la fuente.
```

Un ancla puede indicar:

- página, sección, párrafo o tabla;
- tiempo inicial y final de audio;
- hablante;
- commit, ruta, símbolo y líneas;
- ID de evento;
- recurso, ambiente e instante de observación.

### 12.4 Hechos y evidencia

Modelo conceptual:

```text
knowledge_item
  kind
  subject
  predicate
  object
  normalized_content
  scope
  status
  authority_level
  confidence
  valid_from / valid_to
  system_from / system_to
  created_by / approved_by
  supersedes

knowledge_evidence
  knowledge_item
  artifact_segment
  role
  exact_anchor
  content_hash
```

Roles de evidencia:

- respalda;
- contradice;
- contextualiza;
- sustituye;
- originó.

Pact conserva contradicciones y no elige silenciosamente una versión.

### 12.5 Grafo temporal

Ejemplos:

```text
requisito → motivó → decisión
decisión → restringe → componente
componente → depende de → servicio
servicio → usa → base de datos
cambio → implementa → requisito
prueba → valida → comportamiento
incidente → fue causado por → despliegue
intención → puede afectar → componente
```

Cada relación registra:

- fuente;
- método de extracción;
- confianza;
- alcance;
- ambiente;
- revisión;
- validez temporal;
- momento en que Pact la conoció;
- evidencia;
- estado.

El modelo debe ser bitemporal:

- **tiempo de validez:** cuándo era cierto en el proyecto;
- **tiempo de sistema:** cuándo Pact lo conoció o corrigió.

Git añade una dimensión de revisión. No se debe tratar un hash como un número lineal entre ramas.

### 12.6 Almacenamiento del grafo

La representación canónica inicial será PostgreSQL:

```text
entities
relations
relation_evidence
```

Las consultas utilizan índices y CTE recursivas. Si los recorridos profundos o análisis masivos justifican una base de grafos, esta recibirá una proyección regenerable. No se crearán dos fuentes editables.

---

## 13. Ingestión de fuentes

### 13.1 Fuentes previstas

- repositorios Git;
- pull requests y revisiones;
- documentos y wikis;
- reuniones, audio y transcripciones;
- sistemas de tickets;
- CRM y solicitudes de clientes;
- correo y mensajería autorizados;
- CI/CD y pruebas;
- observabilidad e incidentes;
- infraestructura y proveedores cloud;
- bases de datos y catálogos de esquema;
- carga manual.

Cada conector declara:

- capacidades;
- permisos solicitados;
- estrategia de sincronización;
- cursor;
- política de eliminación;
- mapeo de ACL;
- tipos de artefacto;
- salud y retraso.

### 13.2 Pipeline común

```text
1. Detectar alta, cambio, eliminación o cambio de permisos.
2. Registrar versión externa y hash.
3. Conservar original inmutable.
4. Importar ACL y clasificación.
5. Extraer estructura y texto.
6. Crear segmentos con anclas estables.
7. Detectar idioma, entidades y alias.
8. Proponer conocimiento y relaciones.
9. Vincular cada propuesta con evidencia.
10. Crear índice textual.
11. Generar embeddings.
12. Detectar duplicados y contradicciones.
13. Publicar eventos.
14. Invalidar proyecciones y contextos afectados.
15. Solicitar revisión cuando corresponda.
```

Idempotencia:

```text
source_id + external_version + content_hash
```

### 13.3 Reuniones y transcripciones

Pact debe:

- registrar consentimiento y participantes;
- conservar hablante y marcas temporales;
- segmentar por temas;
- distinguir discusión, propuesta y decisión;
- extraer solicitudes, compromisos, responsables, fechas y desacuerdos;
- mantener preguntas abiertas;
- permitir correcciones sin destruir versiones previas;
- aplicar retención y clasificación;
- enviar decisiones sensibles a revisión humana.

“Quizás eliminemos esta API” no significa “se decidió eliminar esta API”.

### 13.4 Documentos

Los parsers deben preservar:

- encabezados;
- páginas;
- párrafos;
- listas;
- tablas;
- comentarios;
- enlaces;
- revisiones.

OCR y transcripción declaran confianza. Pact no cita una extracción sin poder mostrar el original o un ancla verificable.

### 13.5 Código

No conviene tratar cada línea como un fragmento genérico. La indexación debe comprender:

- repositorios y revisiones;
- archivos;
- símbolos y firmas;
- imports y llamadas;
- módulos y componentes;
- contratos y esquemas;
- endpoints;
- pruebas;
- commits y diffs;
- manifiestos;
- infraestructura declarativa.

Los embeddings de código deben identificarse y evaluarse por separado de los de lenguaje natural.

### 13.6 Invalidación

```text
Fuente modificada
    ↓
Crear nueva versión
    ↓
Identificar segmentos afectados
    ↓
Expirar conocimiento dependiente
    ↓
Regenerar índices y extracción
    ↓
Detectar contradicciones
    ↓
Invalidar context packets
    ↓
Notificar intenciones activas afectadas
```

Los paquetes de contexto registran:

- cursor de eventos;
- revisión Git;
- ambiente;
- versión de políticas;
- identidad y permisos;
- versión de índices;
- fecha;
- TTL;
- retraso de ingestión.

---

## 14. Búsqueda vectorial y recuperación híbrida

### 14.1 Papel de pgvector

pgvector forma parte de la arquitectura completa desde el comienzo, aunque la cobertura de embeddings crezca progresivamente.

Debe permitir:

- búsqueda exacta para corpus pequeños o consultas críticas;
- HNSW para volumen y baja latencia;
- separación por modelo, versión y dimensión;
- distancia configurable;
- reindexación incremental;
- evaluación de recall;
- búsquedas híbridas.

pgvector admite búsqueda de vecinos exacta y aproximada, índices HNSW e IVFFlat y combinación con full-text search. Véase la [documentación oficial de pgvector](https://github.com/pgvector/pgvector).

### 14.2 Lo que un vector no demuestra

La similitud vectorial no determina:

- vigencia;
- autoridad;
- permiso;
- relación causal;
- exactitud;
- identidad;
- compatibilidad;
- aprobación.

Por eso los vectores son una proyección de recuperación, no una memoria canónica.

### 14.3 Pipeline de embeddings

Cada embedding registra:

```text
segment_id
model_provider
model_name
model_version
dimensions
distance_metric
input_hash
generated_at
classification
```

Reglas:

- no mezclar dimensiones incompatibles en el mismo índice;
- heredar ACL y clasificación;
- no generar embeddings de secretos;
- deduplicar por hash;
- soportar reembebido por modelo;
- mantener versión anterior hasta validar la nueva;
- borrar derivados cuando se elimina la fuente.

### 14.4 Recuperación híbrida

```text
1. Autenticar actor, sesión, delegación e intención.
2. Calcular fuentes y clasificaciones autorizadas.
3. Interpretar entidades, periodo, revisión y facetas.
4. Aplicar filtros exactos.
5. Ejecutar búsqueda textual.
6. Ejecutar búsqueda vectorial.
7. Expandir el grafo.
8. Incorporar eventos recientes y trabajo activo.
9. Fusionar rankings.
10. Reordenar opcionalmente.
11. Aplicar vigencia, autoridad, confianza y recencia.
12. Diversificar.
13. Agrupar evidencias favorables y contradictorias.
14. Verificar permisos nuevamente.
15. Compilar dentro del presupuesto.
```

PostgreSQL incluye operadores y funciones para full-text search mediante `tsvector` y `tsquery`. Véase la [documentación oficial](https://www.postgresql.org/docs/current/functions-textsearch.html).

### 14.5 Recuperación segura

El permiso se aplica antes de construir candidatos. Medidas:

- filtros por organización y proyecto;
- particiones o índices parciales para dominios sensibles;
- RLS como defensa adicional;
- búsquedas exactas sobre conjuntos autorizados pequeños;
- cachés separadas por actor, política y snapshot;
- pruebas de no filtración;
- invalidación inmediata ante revocación.

Una base vectorial especializada puede incorporarse cuando el volumen, la distribución regional o la latencia lo exijan. Se conectará mediante una interfaz `VectorIndex` y seguirá siendo regenerable desde PostgreSQL y las fuentes.

---

## 15. Compilador de contexto

### 15.1 Contrato

Entrada:

```json
{
  "project_id": "prj_01",
  "actor_session_id": "ses_01",
  "intent": {
    "goal": "Modificar la renovación de sesiones",
    "operation": "code_change"
  },
  "scope": {
    "environment": "staging",
    "git_revision": "81af23"
  },
  "facets": [
    "decisions",
    "constraints",
    "code",
    "infrastructure",
    "active_work",
    "required_validations"
  ],
  "budget": {
    "max_tokens": 12000
  },
  "freshness": {
    "at_least_event_cursor": 84920
  }
}
```

Salida:

```json
{
  "snapshot": {
    "event_cursor": 84931,
    "git_revision": "81af23",
    "policy_version": "p-17",
    "generated_at": "2026-07-25T10:30:00Z",
    "index_lag_ms": 320
  },
  "intent": {},
  "facts": [],
  "decisions": [],
  "constraints": [],
  "components": [],
  "infrastructure": [],
  "active_work": [],
  "recent_changes": [],
  "open_questions": [],
  "contradictions": [],
  "required_validations": [],
  "evidence": [],
  "warnings": [],
  "omitted_due_to_budget": [],
  "expires_at": "2026-07-25T11:00:00Z"
}
```

### 15.2 Tipos de paquete

- contexto para implementar un cambio;
- preparación de reunión;
- antecedentes de una decisión;
- investigación de incidente;
- cambio de infraestructura;
- onboarding;
- análisis de impacto;
- handoff entre agentes;
- revisión;
- despliegue.

### 15.3 Contexto determinista y síntesis

Pact construye primero el objeto estructurado. Después una IA puede:

- resumir;
- adaptar el nivel técnico;
- proponer un orden;
- explicar contradicciones;
- seleccionar ejemplos.

Cada afirmación narrativa debe referenciar IDs de evidencia. Si no existe evidencia suficiente, se expresa incertidumbre.

### 15.4 Deltas de contexto

Los agentes activos pueden suscribirse a cambios relevantes:

```text
decision.superseded
constraint.added
active_work.overlap
git.base_changed
infrastructure.drift_detected
permission.revoked
```

Pact mantiene un snapshot estable y envía deltas, evitando reconstruir continuamente todo el prompt.

### 15.5 Consistencia solicitada

Modos:

```text
eventual
at_least_event_cursor
strict_for_required_sources
```

Si una fuente crítica todavía no fue indexada, Pact espera o declara de forma explícita que no puede garantizar contexto actualizado.

---

## 16. Papel de la inteligencia artificial

### 16.1 Arquitectura sin una IA obligatoria

Pact debe funcionar aunque ningún modelo esté disponible:

- autenticación;
- sesiones;
- intenciones;
- leases;
- eventos;
- políticas;
- aprobaciones;
- Git;
- runners;
- auditoría;
- búsquedas estructuradas.

Las capacidades probabilísticas se añaden detrás de interfaces explícitas.

### 16.2 Roles posibles

#### Context curator

- resume evidencia;
- identifica contenido redundante;
- propone hechos y decisiones;
- detecta posible obsolescencia;
- nunca acepta sus propias propuestas.

#### Scope and impact analyst

- infiere componentes afectados;
- relaciona cambios con contratos;
- propone validaciones;
- declara confianza y evidencia.

#### Planner

- descompone una intención;
- propone dependencias;
- recomienda agentes o capacidades;
- no concede permisos.

#### Conflict analyst

- explica conflictos;
- propone resoluciones;
- compara objetivos;
- no bloquea por sí solo una integración.

#### Meeting analyst

- extrae solicitudes, decisiones candidatas y compromisos;
- conserva hablantes y anclas;
- distingue propuesta de aceptación.

#### Infrastructure advisor

- interpreta planes;
- explica riesgo;
- propone mitigaciones;
- no ejecuta ni aprueba.

#### Verifier

- contrasta resultado e intención;
- busca evidencia faltante;
- propone pruebas;
- no sustituye validaciones deterministas.

### 16.3 Orquestador

Un orquestador es otro actor de Pact con capacidades limitadas. Puede:

- crear intenciones hijas;
- asignar agentes;
- solicitar contexto;
- pedir validaciones;
- proponer prioridades.

No puede:

- poseer secretos permanentes;
- modificar políticas superiores;
- aprobar su propio cambio sensible;
- convertir inferencias en hechos;
- saltarse la integration queue;
- declarar éxito sin evidencias.

Puede haber varios orquestadores especializados. El estado del proyecto no vive dentro de su ventana de contexto.

### 16.4 Abstracción de proveedores y modelos

Registrar:

- proveedor;
- modelo;
- versión;
- capacidades;
- región;
- política de datos;
- coste;
- latencia;
- ventana de contexto;
- tipos de entrada permitidos;
- evaluación aprobada;
- fecha de retiro.

El routing considera:

- sensibilidad;
- tarea;
- idioma;
- coste;
- latencia;
- calidad evaluada;
- disponibilidad;
- residencia.

### 16.5 Salidas estructuradas

Los workers de IA deben producir esquemas validados:

```json
{
  "claim": "La rotación de sesiones afecta al endpoint X",
  "claim_type": "inferred_relation",
  "confidence": 0.78,
  "evidence_ids": ["seg_1", "sym_9"],
  "scope": {
    "git_revision": "81af23"
  }
}
```

Una respuesta que no valida el esquema no modifica el conocimiento.

### 16.6 Coste y presupuesto

Cada proyecto define:

- presupuesto mensual;
- límite por intención;
- modelos permitidos;
- prioridad;
- caché;
- reintentos;
- operaciones que requieren aprobación por coste.

Pact registra tokens, latencia y utilidad, sin guardar prompts secretos innecesarios.

### 16.7 Evaluación previa a promoción

Un nuevo modelo, prompt o estrategia de retrieval:

1. se ejecuta contra un conjunto de evaluación;
2. opera en shadow mode;
3. compara citas, cobertura, seguridad, latencia y coste;
4. requiere aprobación para ser predeterminado;
5. conserva rollback.

---

## 17. Identidad, delegación y autorización

### 17.1 Cadena de identidad

```text
Persona o servicio responsable
        ↓ delegación
Instancia de agente
        ↓ autenticación
Sesión
        ↓ asignación
Intención
        ↓ concesión
Capacidad temporal
        ↓ solicitud
Acción
```

Una persona y su agente nunca son el mismo actor en auditoría.

### 17.2 Autenticación

#### Humanos

- OIDC para instalaciones compartidas;
- autenticación local segura en modo personal;
- MFA o step-up para acciones sensibles;
- futura sincronización de grupos mediante SCIM;
- sesiones revocables.

OpenID Connect define una capa de identidad sobre OAuth 2.0. Véase la [especificación oficial](https://openid.net/specs/openid-connect-core-1_0-errata2.html).

#### Agentes

- identidad estable opcional;
- instancia y sesión únicas;
- token de corta duración;
- patrocinador humano o de servicio;
- capacidades atenuadas;
- audiencia y proyecto limitados;
- revocación inmediata.

OAuth Token Exchange incluye semántica de delegación donde actor y sujeto permanecen identificables por separado; puede inspirar el intercambio de credenciales de Pact. Véase [RFC 8693](https://datatracker.ietf.org/doc/rfc8693/).

#### Nodos y runners

- inscripción de un solo uso;
- identidad de workload;
- mTLS;
- rotación automática;
- etiquetas de ambiente;
- attestation cuando sea viable;
- revocación.

SPIFFE define identidades portables de workload y certificados de corta duración para procesos. Es una integración futura adecuada para instalaciones avanzadas. Véanse los [conceptos oficiales de SPIFFE](https://spiffe.io/docs/latest/spiffe/concepts/).

### 17.3 Modelo de autorización

Combinación:

- **RBAC:** roles organizativos estables;
- **ABAC:** condiciones dinámicas;
- **capabilities:** delegación concreta y temporal;
- **policies:** restricciones y obligaciones;
- **approvals:** consentimiento asociado a una acción exacta.

Factores ABAC:

- organización;
- proyecto;
- actor y patrocinador;
- intención;
- ambiente;
- recurso y criticidad;
- acción;
- datos;
- revisión;
- validaciones;
- horario;
- runner;
- coste;
- radio de impacto;
- reversibilidad.

### 17.4 Capacidad

```yaml
subject: agent-session-7
sponsor: user-jorge
project: pact
actions:
  - infra.plan
  - service.restart
resources:
  environment: staging
  selectors:
    - service: api
conditions:
  expires_at: 2026-07-25T18:00:00Z
  allowed_runners:
    - staging-runner
  max_uses: 5
  delegation_depth: 0
prohibitions:
  - secret.reveal
  - infra.delete
  - iam.modify
```

Propiedades:

- corta duración;
- audiencia limitada;
- revocable;
- vinculada a sesión;
- atenuación obligatoria al delegar;
- no transferible;
- de un solo uso para operaciones críticas;
- validación inmediatamente antes de ejecutar.

### 17.5 Riesgo

| Nivel | Ejemplo | Tratamiento |
|---|---|---|
| R0 | Leer metadatos generales | Automático |
| R1 | Consultar logs filtrados | Automático o supervisado |
| R2 | Reiniciar desarrollo | Validación y auditoría |
| R3 | Desplegar staging o cambio reversible en producción | Plan, pruebas y aprobación |
| R4 | Cambiar IAM, red, secretos o datos productivos | Separación de funciones, MFA y aprobación múltiple |
| R5 | Revelar claves maestras, desactivar auditoría o conceder admin permanente | Prohibido por política normal |

El nivel se calcula mediante factores explícitos. Una IA puede explicar el riesgo, pero no asignarlo libremente.

### 17.6 Aprobaciones

Una aprobación se vincula a:

- hash de acción;
- intención;
- revisión;
- plan;
- variables no secretas;
- recursos;
- versión de política;
- ventana temporal.

Si cambia cualquiera de estos elementos, la aprobación deja de ser válida.

La pantalla de aprobación muestra:

- solicitante y patrocinador;
- qué cambiará;
- creaciones, modificaciones y destrucciones;
- ambiente;
- coste estimado;
- reversibilidad;
- validaciones;
- evidencia;
- riesgo;
- tiempo de expiración.

### 17.7 Motor de políticas

Jerarquía:

```text
Base de Pact
  → organización
    → proyecto
      → ambiente
        → recurso
```

Una capa inferior puede restringir, no eliminar prohibiciones superiores.

Resultados:

- permitir;
- denegar;
- exigir validaciones;
- exigir aprobaciones;
- seleccionar runner;
- limitar credencial;
- exigir respaldo;
- imponer ventana;
- reducir alcance.

Las políticas son:

- versionadas;
- probadas;
- simulables;
- revisadas;
- protegidas del actor evaluado;
- registradas junto con cada decisión.

---

## 18. Secretos y credenciales

### 18.1 Regla principal

> El permiso de usar un secreto no implica permiso para verlo.

PostgreSQL guarda:

- `secret_reference`;
- proveedor;
- rol o path;
- recurso;
- clasificación;
- política;
- metadatos de rotación;
- lease ID;
- emisión, expiración y revocación.

No guarda en texto claro:

- claves SSH;
- tokens cloud;
- contraseñas;
- claves API;
- claves privadas.

### 18.2 Sistemas de custodia

Adaptadores:

- Vault u OpenBao;
- gestores de secretos cloud;
- KMS/HSM;
- keychain local;
- identidad nativa de Kubernetes;
- OAuth de servicios externos;
- autoridades de certificados SSH.

Vault puede generar credenciales dinámicas con TTL y revocación, evitando usuarios compartidos permanentes. Véase la [documentación de credenciales dinámicas](https://developer.hashicorp.com/vault/docs/secrets/databases).

### 18.3 Flujo

```text
1. El agente solicita una acción.
2. Pact evalúa capacidad, política e intención.
3. Se obtienen aprobaciones.
4. Pact selecciona runner.
5. El runner prueba su identidad.
6. El broker solicita credencial efímera.
7. Se monta en memoria o se entrega al subproceso.
8. Se ejecuta la acción.
9. Se revoca o expira.
10. Pact conserva solo metadatos y evidencia filtrada.
```

### 18.4 Medidas obligatorias

- nunca insertar secretos en prompts;
- redacción defensiva de logs;
- egress de red limitado;
- variables efímeras o archivos temporales protegidos;
- rotación;
- revocación;
- detección de exposición;
- exclusión de embeddings;
- auditoría sin valores;
- cifrado por envoltura si Pact debe custodiar material propio;
- claves maestras fuera de PostgreSQL.

El filtrado de logs es una segunda defensa. La protección principal es no entregar el secreto al proceso que no lo necesita.

---

## 19. Infraestructura y ejecución

### 19.1 Fuentes de verdad

```text
Git
  Estado deseado

Backend IaC
  Mapeo técnico, estado y locking

Proveedor real
  Estado observado

Pact
  Intención, coordinación, autorización, relaciones y auditoría

Gestor de secretos
  Credenciales
```

### 19.2 Protocolo abstracto de infraestructura

Operaciones:

```text
discover
validate
plan
estimate
policy_check
apply
refresh
detect_drift
import
reconcile
rollback_or_compensate
```

Adaptadores:

- Terraform;
- OpenTofu;
- Kubernetes;
- GitOps;
- proveedores cloud;
- Ansible;
- SSH controlado;
- migraciones de base de datos;
- plataformas de despliegue.

El protocolo no depende de una sola herramienta.

### 19.3 Terraform y OpenTofu

Flujo:

1. Crear intención.
2. Trabajar sobre revisión aislada.
3. Formatear, validar y probar.
4. Fijar proveedores y módulos.
5. Generar plan en runner.
6. Extraer representación estructurada y filtrada.
7. Clasificar creación, cambio, reemplazo y destrucción.
8. Estimar coste y riesgo.
9. Evaluar políticas.
10. Aprobar hash exacto.
11. Integrar IaC.
12. Regenerar plan sobre commit integrado.
13. Aplicar exactamente el plan aprobado.
14. Capturar evidencia.
15. Refrescar estado y detectar drift.

El estado remoto necesita cifrado, locking, versionado y permisos. No se copia indiscriminadamente a la base de conocimiento. Terraform advierte que los estados y planes pueden contener valores sensibles y recomienda proteger el estado fuera de Git. Véase [gestión de datos sensibles](https://developer.hashicorp.com/terraform/language/manage-sensitive-data) y [propósito y almacenamiento del estado](https://developer.hashicorp.com/terraform/language/state).

### 19.4 Kubernetes

Se prefiere GitOps:

```text
Agente propone manifiesto
→ validación
→ política
→ integración
→ reconciliador aplica
→ Pact observa rollout y salud
```

Las operaciones directas quedan tipadas, restringidas y auditadas. El drift se detecta.

### 19.5 Servidores

Preferencias:

- despliegues inmutables;
- administración declarativa;
- runners dentro de la red;
- conexión saliente;
- certificados SSH temporales;
- comandos tipados;
- sesiones grabadas en operaciones sensibles;
- sin claves permanentes en Pact.

Shell arbitrario será un escape de alto riesgo, nunca el camino predeterminado.

### 19.6 Bases de datos

Capacidades separadas:

- inspeccionar esquema;
- ejecutar lectura limitada;
- acceder a datos sensibles;
- aplicar migración;
- administrar usuarios;
- restaurar respaldo.

Las migraciones requieren:

- versión;
- intención;
- lock;
- validaciones;
- plan;
- respaldo según riesgo;
- estrategia de recuperación;
- evidencia del resultado.

### 19.7 Estados de un trabajo

```text
requested
authorized
awaiting_approval
queued
leased
executing
succeeded
failed
canceled
expired
outcome_unknown
reconciling
```

Una pérdida de conexión durante una mutación produce `outcome_unknown`, no `failed`. Pact consulta el recurso antes de reintentar.

### 19.8 Runners

Separación por confianza:

- local;
- CI;
- desarrollo;
- staging;
- producción;
- infraestructura;
- datos.

Un runner de desarrollo no puede convertirse dinámicamente en uno de producción.

Requisitos:

- identidad propia;
- attestation opcional;
- certificado rotatorio;
- etiquetas firmadas;
- conexión saliente;
- pull de trabajos;
- nonce y anti-replay;
- aislamiento;
- límites;
- red denegada por defecto;
- credenciales temporales;
- recolección de evidencia;
- destrucción del entorno.

Niveles de aislamiento:

- proceso restringido;
- contenedor sin privilegios;
- microVM;
- ejecutor dedicado.

La política elige el nivel según riesgo.

### 19.9 Acciones tipadas

Cada acción define:

- esquema de entrada;
- esquema de salida;
- precondiciones;
- postcondiciones;
- idempotencia;
- riesgo;
- evidencias;
- cancelación;
- reconciliación;
- compensación posible.

Ejemplos:

```text
infra.plan
infra.apply
deployment.rollout
service.restart
logs.query
database.query_readonly
database.migration_apply
ssh.command_execute
git.change_integrate
```

---

## 20. Auditoría, observabilidad y evidencia

### 20.1 Preguntas que debe responder

```text
Quién pidió qué
en nombre de quién
con qué intención
qué autoridad poseía
qué política se aplicó
quién aprobó
qué entradas se usaron
qué runner actuó
qué credencial temporal fue emitida
qué ocurrió
qué evidencia lo demuestra
```

### 20.2 Evidencia

Ejemplos:

- hash de ChangeSet;
- plan de infraestructura;
- resultados de pruebas;
- diff;
- identificador del proveedor;
- salida filtrada;
- estado antes y después;
- artefacto de compilación;
- firma del runner;
- comprobación posterior.

Objetos grandes viven en object storage cifrado y direccionado por hash.

### 20.3 Auditoría resistente a alteración

- eventos append-only;
- roles sin permiso de actualización;
- hashes encadenados opcionales;
- exportación periódica;
- almacenamiento WORM en instalaciones que lo requieran;
- integración SIEM;
- alertas;
- retención definida.

Una cadena de hashes detecta alteraciones; no sustituye permisos ni una copia externa.

### 20.4 Observabilidad

Instrumentar:

- trazas;
- métricas;
- logs estructurados;
- correlation ID;
- actor, proyecto e intención;
- latencia de contexto;
- retraso de índices;
- colas;
- errores;
- costes;
- salud de conectores;
- uso de capacidades;
- resultados de políticas.

OpenTelemetry proporciona un modelo común para correlacionar señales como logs y trazas. Véase la [documentación oficial](https://opentelemetry.io/docs/concepts/signals/logs/).

Los datos de observabilidad también pueden contener información sensible y deben clasificarse y filtrarse.

---

## 21. Experiencia de usuario y herramientas

### 21.1 Principio de adopción

Pact debe aportar valor antes de exigir gobierno estricto. Un equipo puede comenzar en modo observador, comprobar el contexto y la coordinación, y aumentar el control de integración e infraestructura cuando confíe en el sistema.

### 21.2 CLI propuesta

Los nombres son provisionales, pero muestran la experiencia deseada.

```text
pact init
pact up
pact down
pact status
pact doctor
pact login

pact project create
pact project connect
pact project export

pact agent register
pact session list
pact intent create
pact intent status
pact intent complete

pact workspace create
pact workspace list
pact workspace open
pact changes submit

pact context get
pact context explain
pact evidence open

pact infra discover
pact infra plan
pact infra apply
pact infra drift

pact approvals list
pact approvals inspect
pact approvals approve

pact audit inspect
pact events tail
```

### 21.3 `pact init`

Debe:

1. detectar repositorio;
2. crear o vincular proyecto;
3. proponer `pact.yaml` como manifiesto compartido y `.pact/` como estado local;
4. detectar stack y herramientas;
5. registrar repositorio;
6. configurar modo de gobierno;
7. iniciar o conectar servidor;
8. validar PostgreSQL y pgvector;
9. instalar integración opcional con proveedor Git;
10. ejecutar diagnóstico;
11. no modificar políticas sensibles sin confirmación.

### 21.4 `pact up`

En modo personal:

- valida Docker o instalación elegida;
- levanta PostgreSQL, Pact Server y almacenamiento local;
- espera migraciones;
- inicia o comprueba Pact Node;
- imprime endpoint y salud;
- no expone servicios a la red pública por defecto.

### 21.5 Interfaz web

#### Project cockpit

- estado general;
- revisión canónica;
- agentes y personas activas;
- intenciones;
- bloqueos;
- conflictos;
- eventos recientes;
- salud de fuentes;
- retraso de índices.

#### Work map

- intenciones y dependencias;
- scopes;
- agentes;
- handoffs;
- ChangeSets;
- integration queue.

#### Knowledge explorer

- búsqueda híbrida;
- hechos;
- decisiones;
- contradicciones;
- evidencia;
- grafo;
- historial temporal;
- fuentes y permisos.

#### Infrastructure map

- ambientes;
- recursos;
- dependencias;
- estado deseado y observado;
- drift;
- planes;
- ejecuciones;
- salud.

#### Access and policy

- miembros;
- agentes;
- delegaciones;
- capacidades activas;
- runners;
- políticas;
- aprobaciones;
- revocaciones.

#### Audit timeline

- filtros por actor, intención, recurso y ambiente;
- cadena causal;
- evidencias;
- exportación.

#### Administration

- fuentes;
- conectores;
- modelos;
- presupuestos;
- retención;
- backups;
- salud;
- migraciones.

### 21.6 SDKs

Prioridad:

1. Go, para componentes y adaptadores internos.
2. TypeScript, para agentes y web.
3. Python, para agentes, análisis e ingestión.

Cada SDK debe incluir:

- cliente tipado;
- autenticación;
- idempotencia;
- reintentos;
- paginación;
- streams;
- manejo de cursores;
- schemas;
- instrumentación;
- helpers para herramientas de agentes.

### 21.7 Adaptadores para agentes

Pact ofrecerá herramientas como:

```text
pact.project.status
pact.session.start
pact.intent.create
pact.intent.get
pact.scope.declare
pact.context.get
pact.context.subscribe
pact.evidence.open
pact.workspace.create
pact.changeset.submit
pact.approval.request
pact.action.request
pact.knowledge.propose
pact.decision.record
```

Un adaptador MCP u otro protocolo de herramientas puede exponerlas, pero la API canónica seguirá siendo Pact.

No se debe ofrecer a cualquier agente un `publish_event` arbitrario. El agente envía comandos de dominio; Pact emite eventos confirmados.

### 21.8 Notificaciones

Canales:

- interfaz;
- stream del agente;
- correo o mensajería mediante conectores;
- webhooks.

Notificaciones relevantes:

- conflicto que afecta a una intención;
- cambio de revisión base;
- aprobación pendiente;
- lease por expirar;
- sesión perdida;
- decisión crítica nueva;
- permiso revocado;
- validación fallida;
- drift;
- resultado desconocido.

Agrupar y priorizar para evitar fatiga.

### 21.9 Exportación y portabilidad

Un proyecto debe poder exportar:

- configuración;
- políticas;
- decisiones y hechos;
- relaciones;
- eventos permitidos;
- referencias de evidencias;
- índices regenerables opcionalmente;
- manifiesto de versiones.

Los formatos deben ser documentados. La salida de una instalación administrada no puede depender de acceso perpetuo al proveedor.

---

## 22. Stack tecnológico objetivo

### 22.1 Decisiones base

| Área | Elección | Motivo |
|---|---|---|
| Núcleo | Go | Binario, concurrencia, red, operación y simplicidad |
| Base canónica | PostgreSQL | Transacciones, concurrencia, búsqueda, extensiones y operación compartida |
| Vectores | pgvector | Mantener búsqueda semántica junto a filtros y datos estructurados |
| Objetos | API compatible con object storage | Portabilidad entre local, self-hosted y cloud |
| API | HTTP + JSON | Accesibilidad y facilidad para SDKs/agentes |
| Streams | SSE y WebSocket | Recuperación durable y canal bidireccional |
| Esquemas | OpenAPI + JSON Schema | Contratos generables y validables |
| Eventos | Perfil compatible con CloudEvents | Interoperabilidad y sobre estándar |
| UI | TypeScript | Ecosistema web y SDK compartido |
| IA y conectores | Go, TypeScript o Python | Elegir por ecosistema sin contaminar el núcleo |
| Políticas | Núcleo propio + adaptador OPA | Garantías básicas y extensibilidad |
| Identidad humana | OIDC | Integración estándar con IdP |
| Workload identity | mTLS; SPIFFE opcional | Identidad rotatoria de nodos y runners |
| IaC | Protocolo abstracto + Terraform/OpenTofu inicial | Valor inmediato sin acoplamiento permanente |
| Observabilidad | OpenTelemetry | Trazas, métricas y logs correlacionados |
| Despliegue local | Docker Compose | Experiencia unificada con PostgreSQL |
| Despliegue avanzado | Contenedores/Kubernetes | Escala y aislamiento cuando sea necesario |

### 22.2 Modular monolith

No comenzar con microservicios.

```text
cmd/
  pact/
  pact-server/
  pact-node/
  pact-runner/

internal/
  identity/
  authorization/
  coordination/
  protocol/
  events/
  scm/
  workspaces/
  knowledge/
  retrieval/
  context/
  infrastructure/
  execution/
  policies/
  approvals/
  audit/
  storage/

schemas/
  openapi/
  jsonschema/
  events/

sdk/
  go/
  typescript/
  python/

web/
connectors/
deploy/
docs/
tests/
```

Un módulo solo se convierte en servicio cuando:

- necesita escalar de manera independiente;
- utiliza una toolchain incompatible;
- requiere aislamiento de seguridad;
- tiene un ciclo de disponibilidad distinto;
- una medición demuestra que el proceso único no basta.

### 22.3 Componentes que no son obligatorios al principio

| Tecnología | Incorporar cuando |
|---|---|
| Bus de eventos dedicado | El outbox y los workers de PostgreSQL no cubran volumen o desacoplamiento |
| Redis | Exista una necesidad medida de caché distribuida o coordinación de alta frecuencia |
| Base vectorial separada | pgvector no alcance volumen, región o latencia |
| Base de grafos | Los recorridos complejos dominen el workload |
| Motor de workflows externo | Las sagas y trabajos duraderos superen al motor interno |
| Motor de búsqueda separado | PostgreSQL full-text no cubra escala o relevancia |
| MicroVMs | El riesgo requiera aislamiento superior a contenedores |
| SPIFFE/SPIRE | Haya múltiples clusters, runners y dominios de confianza |
| Bus multi-región | Exista despliegue distribuido real |

Excluirlos inicialmente no significa descartarlos. Todas estas funciones deben estar detrás de interfaces y métricas que permitan sustituir la implementación.

### 22.4 Dependencias internas importantes

Interfaces:

```text
EventPublisher
ObjectStore
VectorIndex
GraphProjection
SearchIndex
SecretProvider
IdentityProvider
PolicyEvaluator
SCMProvider
InfrastructureProvider
RunnerBackend
ModelProvider
DocumentConnector
```

Las interfaces no deben diseñarse como abstracciones universales vacías. Se definen a partir de casos concretos y una semántica canónica de Pact.

---

## 23. Seguridad y modelo de amenazas

### 23.1 Fronteras de confianza

```text
Internet
  ↕
Pact API
  ↕
Control plane
  ↕
PostgreSQL / object storage / KMS

Control plane
  ↕ conexión autenticada
Node o runner
  ↕ frontera de sandbox y red
Herramienta
  ↕ credencial temporal
Recurso externo
```

Cada cruce requiere autenticación, autorización, validación y auditoría.

### 23.2 Invariantes de seguridad

1. El contenido recuperado nunca concede permisos.
2. Un agente no hereda automáticamente autoridad humana.
3. Toda delegación reduce o conserva alcance, nunca lo amplía.
4. El permiso de usar un secreto no implica verlo.
5. Toda mutación tiene actor, patrocinador, intención, revisión y política.
6. La autorización se reevalúa justo antes de ejecutar.
7. La aprobación se vincula al hash exacto.
8. Producción falla de forma cerrada.
9. Las credenciales son temporales y revocables.
10. Los ambientes usan identidades y runners separados.
11. Los agentes y runners no pueden alterar su auditoría.
12. Los secretos no entran en prompts, vectores ni logs.
13. Una inferencia no autoriza.
14. Un resultado desconocido no se reintenta a ciegas.
15. Pact detecta cambios externos.

### 23.3 Amenazas y controles

| Amenaza | Controles |
|---|---|
| Prompt injection desde código o documento | Contenido no confiable, herramientas tipadas y autorización fuera del modelo |
| Agente comprometido | Capacidades mínimas, TTL, sandbox, egress y revocación |
| Robo o replay de token | mTLS/DPoP donde aplique, nonce, audiencia, TTL y uso único |
| Escalación de delegación | Atenuación, profundidad máxima y techo de política |
| Fuga de secretos | Uso sin revelación, credenciales efímeras y no indexación |
| Escape del runner | Sandbox, no privilegios, sin socket del host, runner efímero |
| Inyección de comandos | Entradas estructuradas; shell restringido |
| Cambio entre plan y apply | Hash, locking, reevaluación y plan guardado |
| Destrucción accidental | Riesgo, aprobaciones, respaldo, cuotas y simulación |
| Bypass de Git o cloud | Protección remota, webhooks, auditoría y reconciliación |
| Envenenamiento de conocimiento | Procedencia, estado, evidencia y revisión |
| Filtración vectorial | ACL previa, separación de tenants, RLS y pruebas |
| Compromiso del control plane | Sin secretos permanentes, KMS, separación de runners |
| Plugin malicioso | Firmas, allowlist, versiones, SBOM y sandbox |
| SSRF o metadata cloud | Egress, destinos permitidos y workload identity |
| Movimiento lateral | Identidades y redes separadas |
| Coste descontrolado | Presupuestos, cuotas y límites |
| Resultado desconocido | Reconciliación antes de reintentar |
| Aprobación engañosa | Diff, riesgo, hash, MFA y separación de funciones |
| Borrado de auditoría | Permisos append-only y exportación externa |
| Cruce entre tenants | Tenant obligatorio, RLS, namespaces y pruebas |

### 23.4 Compromiso de Pact Server

Es el objetivo de mayor valor. Para reducir impacto:

- no almacenar credenciales permanentes;
- separar runners y cuentas por ambiente;
- aplicar política local mínima en runners;
- mantener claves de firma en KMS/HSM;
- utilizar credenciales muy limitadas;
- exigir múltiples aprobaciones para operaciones críticas;
- permitir despliegue autohospedado;
- exportar auditoría;
- detectar comportamiento anómalo.

No se puede afirmar que el riesgo desaparece. Se limita su radio.

### 23.5 Break-glass

Flujo:

1. incidente activo;
2. humano con MFA reciente;
3. motivo;
4. alcance y duración mínimos;
5. aprobación adicional cuando sea posible;
6. runner dedicado;
7. credencial temporal;
8. grabación;
9. notificación;
10. revocación;
11. revisión posterior.

Break-glass no desactiva auditoría ni entrega claves maestras.

### 23.6 Seguridad de la cadena de suministro

- dependencias fijadas;
- actualización automatizada revisada;
- SBOM;
- firma de binarios e imágenes;
- procedencia de builds;
- escaneo;
- publicación reproducible cuando sea viable;
- plugins firmados;
- catálogo de adaptadores confiables;
- runner verifica firma antes de ejecutar.

---

## 24. Requisitos operativos

### 24.1 Disponibilidad y degradación

Si Pact Server no está disponible:

- los workspaces locales pueden continuar;
- el nodo conserva eventos pendientes;
- no se conceden nuevas leases globales;
- no se integran cambios canónicos;
- no se ejecutan acciones exclusivas o productivas;
- al reconectar se reconcilian revisiones y scopes.

Pact no debe crear dos autoridades durante una partición.

### 24.2 Objetivos iniciales de servicio

Objetivos orientativos que deben validarse:

- ningún evento confirmado perdido;
- acuse de comando p95 menor a 250 ms en la misma región;
- propagación de evento p95 menor a 1 s;
- consulta estructurada de estado p95 menor a 500 ms;
- context packet sin IA p95 menor a 2 s para corpus normal;
- contexto con síntesis sujeto al proveedor, con timeout explícito;
- revocación efectiva en segundos;
- visibilidad del retraso de cada índice;
- recuperación verificable desde backup.

No se sacrificará corrección o aislamiento para cumplir una cifra.

### 24.3 Límites y cuotas

- sesiones por actor;
- agentes simultáneos;
- heartbeats;
- eventos;
- trabajos;
- almacenamiento;
- embeddings;
- llamadas de IA;
- ejecuciones;
- coste de infraestructura;
- consultas sensibles.

Las cuotas se aplican por organización, proyecto, actor e intención.

### 24.4 Retención

Políticas distintas para:

- eventos de dominio;
- auditoría;
- señales;
- workspaces;
- artefactos;
- logs;
- transcripciones;
- embeddings;
- context packets;
- prompts y respuestas;
- datos personales;
- evidencias regulatorias.

La eliminación de una fuente debe propagarse a segmentos, vectores, cachés y contextos derivados, salvo una retención legal explícita.

---

## 25. Mapa completo de construcción

Esta sección no reduce la visión a fases. Enumera todos los frentes que deben construirse para materializarla. El orden de dependencias aparece después.

### W00. Descubrimiento y especificación del producto

**Construir**

- glosario normativo;
- escenarios y journeys;
- límites de autoridad;
- principios e invariantes;
- modelo de amenazas inicial;
- ADRs;
- especificación de compatibilidad;
- definición de métricas de valor.

**Cómo**

- convertir escenarios reales en secuencias actor–intención–acción–evidencia;
- mantener un registro de decisiones;
- validar conceptos con prototipos de protocolo antes de automatizar;
- distinguir explícitamente estado declarado, observado, inferido y verificado.

**Terminado cuando**

- dos implementadores pueden describir igual las entidades principales;
- cada sistema de autoridad está definido;
- los escenarios críticos tienen resultados esperados;
- no quedan términos centrales con significados contradictorios.

### W01. Fundación del repositorio y toolchain

**Construir**

- monorepo;
- módulos Go;
- convenciones;
- linting y formato;
- pruebas;
- generación de schemas y clientes;
- CI;
- análisis de seguridad;
- versionado y releases;
- imágenes y binarios firmados.

**Cómo**

- Makefile o task runner pequeño;
- migraciones versionadas;
- tests herméticos;
- hooks opcionales, nunca única garantía;
- builds reproducibles cuando sea viable;
- SBOM y checksum por release.

**Terminado cuando**

- un contribuidor nuevo puede compilar y ejecutar la suite;
- CI produce binarios e imágenes;
- los schemas generados no se modifican manualmente;
- existe procedimiento de release y rollback.

### W02. Especificación y conformidad del protocolo

**Construir**

- OpenAPI;
- JSON Schemas;
- perfil de eventos;
- errores;
- paginación;
- idempotencia;
- cursores;
- negociación de capacidades;
- versionado;
- suite de conformidad.

**Cómo**

- diseñar desde los casos de uso;
- validar todos los payloads;
- generar SDKs;
- publicar ejemplos;
- probar clientes anteriores contra servidores nuevos.

**Terminado cuando**

- una implementación cliente externa puede interactuar sin conocer el código interno;
- duplicar comandos no duplica efectos;
- un cliente reconecta desde cursor;
- las reglas de compatibilidad tienen pruebas.

### W03. Plataforma de persistencia, eventos y trabajos

**Construir**

- PostgreSQL y pgvector;
- migraciones;
- tenancy;
- transacciones;
- idempotency store;
- events;
- outbox;
- jobs;
- retries;
- dead-letter;
- proyecciones;
- backup y restore.

**Cómo**

- transacción única para estado, evento y outbox;
- consumidores idempotentes;
- `LISTEN/NOTIFY` como optimización;
- índices observados;
- RLS en tablas sensibles;
- pruebas de migración hacia delante y atrás cuando sea posible.

**Terminado cuando**

- no se pierden eventos confirmados bajo fallos simulados;
- los workers recuperan trabajos;
- la restauración reconstruye un proyecto;
- el aislamiento entre organizaciones está probado.

### W04. Identidad, organizaciones y sesiones

**Construir**

- organizaciones;
- proyectos;
- principals;
- humanos;
- agentes;
- nodos;
- runners;
- membresías;
- login local;
- OIDC;
- sesiones;
- heartbeats;
- revocación.

**Cómo**

- separar identidad estable de sesión temporal;
- tokens cortos;
- rotación;
- MFA/step-up para riesgo;
- auditoría de login;
- denegación por defecto.

**Terminado cuando**

- dos agentes de la misma persona son distinguibles;
- expirar una sesión revoca su autoridad temporal;
- un proyecto no puede consultar otro;
- funcionan login local y OIDC compartido.

### W05. Delegaciones, capacidades, políticas y aprobaciones

**Construir**

- grants;
- capabilities;
- atenuación;
- revocation list;
- RBAC/ABAC;
- risk engine;
- policy bundles;
- simulación;
- approvals;
- separación de funciones.

**Cómo**

- capacidad opaca y ligada a sesión;
- evaluación antes de solicitar y antes de ejecutar;
- aprobación vinculada a hash;
- políticas versionadas con tests;
- adaptador OPA para reglas avanzadas.

**Terminado cuando**

- un agente no puede ampliar una delegación;
- cambiar un plan invalida su aprobación;
- revocar bloquea nuevas acciones;
- el historial muestra política y versión exactas.

### W06. Coordinación de agentes

**Construir**

- intenciones jerárquicas;
- tareas;
- assignments;
- dependencias;
- scopes;
- overlap;
- leases;
- fencing;
- conflictos;
- señales;
- handoff;
- recuperación de huérfanos.

**Cómo**

- estados explícitos;
- scopes declarados, observados e inferidos;
- leases con TTL del servidor;
- claims consultivos para código;
- exclusividad solo para efectos compartidos;
- señales temporales promovidas explícitamente a conocimiento.

**Terminado cuando**

- varios agentes coordinan una intención;
- una sesión perdida no borra el trabajo;
- una lease antigua no puede ejecutar;
- los solapamientos se notifican sin bloquear injustificadamente.

### W07. Pact Node

**Construir**

- instalación;
- registro;
- conexión saliente;
- protocolo node-server;
- observador local;
- cola offline;
- diagnóstico;
- control de directorios;
- logs filtrados;
- actualización.

**Cómo**

- binario Go;
- socket local para agentes;
- permisos mínimos;
- almacenamiento local cifrado cuando corresponda;
- resync por cursor y revisión;
- health report.

**Terminado cuando**

- varios chats locales usan el mismo proyecto;
- una desconexión conserva trabajo y eventos pendientes;
- reconectar no crea estado contradictorio;
- el nodo no accede fuera de rutas autorizadas.

### W08. Workspaces y Git

**Construir**

- repository bindings;
- detección Git;
- worktrees;
- workspace lifecycle;
- observación de diffs;
- ChangeSets inmutables;
- provider adapters;
- webhooks;
- integration queue;
- modos observer/managed/strict;
- reconciliación externa.

**Cómo**

- utilizar el binario Git y formatos porcelain;
- worktree por ejecución;
- base revision exacta;
- validaciones vinculadas al hash;
- compare-and-swap sobre ref canónica;
- saga para Git y Pact;
- protección de rama en modo estricto.

**Terminado cuando**

- dos agentes no escriben en el mismo worktree;
- carreras de integración no pierden cambios;
- un push externo invalida lo necesario;
- Pact nunca declara integrado antes de confirmar Git.

### W09. Validación e inteligencia de código

**Construir**

- catálogo de lenguajes;
- parser adapters;
- símbolos;
- imports;
- llamadas;
- contratos;
- esquemas;
- test graph;
- build graph;
- análisis de impacto;
- conflictos estructurales.

**Cómo**

- adaptadores incrementales;
- Tree-sitter/LSP cuando resulte apropiado;
- manifests y herramientas nativas;
- revisionar todos los resultados;
- distinguir análisis exacto de inferencia.

**Terminado cuando**

- una modificación de contrato identifica consumidores;
- los resultados pueden reconstruirse por revisión;
- la pérdida de un índice no pierde información canónica;
- se mide precisión de conflictos.

### W10. Registro de fuentes e ingestión

**Construir**

- source registry;
- connector SDK;
- webhooks;
- polling;
- object storage;
- versionado;
- ACL sync;
- parsers;
- OCR;
- transcripción;
- segmentación;
- deduplicación;
- borrado propagado.

**Cómo**

- interfaz por conector;
- cursor;
- original inmutable;
- hash;
- pipeline idempotente;
- clasificación antes de indexar;
- observabilidad por fuente.

**Terminado cuando**

- un documento puede llegar al índice conservando estructura y permisos;
- una modificación crea nueva versión;
- una eliminación retira derivados;
- cada segmento abre su evidencia.

### W11. Modelo de conocimiento y grafo temporal

**Construir**

- entidades;
- alias;
- hechos;
- decisiones;
- requisitos;
- restricciones;
- evidencia;
- relaciones;
- contradicciones;
- aprobación;
- bitemporalidad;
- alcance por revisión y ambiente;
- invalidación.

**Cómo**

- ontología pequeña y extensible;
- estados explícitos;
- evidencia obligatoria;
- propuestas de IA no aceptadas automáticamente;
- relaciones canónicas en PostgreSQL;
- proyección opcional a graph store.

**Terminado cuando**

- Pact distingue discusión, propuesta y decisión;
- una decisión sustituida sigue siendo auditable;
- puede reconstruirse qué se sabía en un momento;
- las contradicciones se muestran.

### W12. Búsqueda textual, vectorial y recuperación

**Construir**

- FTS;
- embedding registry;
- pgvector;
- exact search;
- HNSW;
- filtros;
- graph expansion;
- fusion;
- reranking;
- diversity;
- ACL prefilter;
- evaluation harness.

**Cómo**

- modelos versionados;
- embeddings heredando permisos;
- Reciprocal Rank Fusion;
- reranker opcional;
- comparación ANN contra exacta;
- shadow index para migraciones.

**Terminado cuando**

- las consultas tienen citas;
- se miden recall, latencia y seguridad;
- cambiar de modelo no rompe la fuente canónica;
- no existe filtración entre proyectos.

### W13. Compilador de contexto

**Construir**

- context request;
- facets;
- budgets;
- consistency modes;
- snapshots;
- evidence map;
- warnings;
- contradictions;
- omissions;
- TTL;
- deltas;
- feedback.

**Cómo**

- salida estructurada primero;
- síntesis opcional;
- snapshot por revisión, cursor y política;
- caché por permisos;
- invalidación dirigida.

**Terminado cuando**

- dos agentes pueden transferir trabajo sin leer sus chats;
- cada afirmación tiene evidencia;
- el paquete declara frescura;
- un cambio crítico produce delta o expiración.

### W14. Plataforma de IA

**Construir**

- provider registry;
- model registry;
- routing;
- structured output;
- budgets;
- prompt templates;
- context curator;
- impact analyst;
- planner;
- conflict analyst;
- evaluator;
- shadow mode.

**Cómo**

- IA como worker;
- entradas clasificadas;
- herramientas limitadas;
- esquemas validados;
- citas obligatorias;
- evaluaciones antes de promoción.

**Terminado cuando**

- el núcleo funciona sin IA;
- sustituir un proveedor no cambia el protocolo;
- ninguna salida probabilística concede autoridad;
- coste y utilidad son observables.

### W15. Catálogo de infraestructura y ambientes

**Construir**

- environments;
- resources;
- relations;
- desired state;
- observed state;
- discovery;
- drift;
- ProjectSnapshot;
- topología;
- criticidad;
- clasificación.

**Cómo**

- IDs estables;
- adaptadores;
- observación periódica;
- diferencias explícitas;
- no copiar secretos o state completo.

**Terminado cuando**

- Pact muestra qué servicio usa qué recurso;
- distingue estado deseado, técnico y real;
- detecta drift;
- vincula recurso con código, decisiones y responsables.

### W16. IaC y planes

**Construir**

- Terraform adapter;
- OpenTofu adapter;
- validate;
- plan;
- JSON plan parser;
- estimate;
- policy check;
- apply;
- refresh;
- evidence;
- state backend integration.

**Cómo**

- runners;
- toolchains fijadas;
- state remoto;
- lock;
- plan aprobado por hash;
- regeneración tras integrar;
- post-check.

**Terminado cuando**

- Pact nunca aplica un plan diferente al aprobado;
- fallos de red producen `outcome_unknown`;
- se observan recursos tras apply;
- no se indexan secretos del state.

### W17. Runners y acciones tipadas

**Construir**

- runner protocol;
- registration;
- identity;
- queues;
- work leases;
- sandbox;
- resource limits;
- network policy;
- typed action SDK;
- cancellation;
- reconciliation;
- evidence capture.

**Cómo**

- conexión saliente;
- nonce;
- mTLS;
- contenedores iniciales;
- microVM o dedicado por riesgo;
- argumentos estructurados;
- sin shell predeterminado.

**Terminado cuando**

- un runner no autorizado no recibe trabajos;
- una tarea no puede salir de su sandbox;
- una operación ambigua se reconcilia;
- la evidencia está vinculada al job exacto.

### W18. Secret broker y accesos

**Construir**

- secret references;
- provider adapters;
- credential leases;
- cloud federation;
- database dynamic users;
- SSH certificates;
- Git app tokens;
- redaction;
- revocation.

**Cómo**

- obtener credencial justo a tiempo;
- montar en memoria;
- limitar audiencia y TTL;
- no devolver valor al agente;
- destruir al finalizar.

**Terminado cuando**

- una IA ejecuta una acción sin recibir la clave;
- revocar una sesión revoca o deja expirar accesos;
- los logs no contienen secretos de casos de prueba;
- cada lease queda auditada sin su valor.

### W19. Interfaces, CLI y SDKs

**Construir**

- CLI;
- web app;
- dashboards;
- approvals;
- knowledge explorer;
- infra map;
- audit;
- Go SDK;
- TypeScript SDK;
- Python SDK;
- adapter de herramientas.

**Cómo**

- API-first;
- clientes generados;
- accesibilidad;
- estados en tiempo real;
- explicaciones y evidencia;
- acciones sensibles con confirmación clara.

**Terminado cuando**

- usuario personal instala y conecta dos agentes;
- equipo observa estado compartido;
- aprobador entiende exactamente qué autoriza;
- un desarrollador externo construye un cliente.

### W20. Despliegue y operación

**Construir**

- Docker Compose;
- instalación local;
- charts/manifests avanzados;
- migraciones;
- health checks;
- backups;
- restores;
- actualizaciones;
- HA;
- scaling;
- region strategy;
- administración.

**Cómo**

- mismo protocolo en todas las modalidades;
- configuración declarativa;
- secretos externos;
- rolling upgrades;
- compatibilidad de schema;
- runbooks.

**Terminado cuando**

- local, self-hosted y administrado ejecutan la misma suite;
- una actualización conserva compatibilidad;
- se restaura una instalación desde cero;
- existen runbooks de incidentes.

### W21. Seguridad, privacidad y cadena de suministro

**Construir**

- threat model continuo;
- tests de autorización;
- tenancy tests;
- secret scanning;
- security headers;
- encryption;
- retention;
- deletion;
- consent;
- export;
- SBOM;
- signing;
- vulnerability management;
- incident response.

**Cómo**

- seguridad en cada PR;
- revisiones especializadas;
- pentesting;
- casos adversariales;
- rotación;
- simulacros;
- privacidad por diseño.

**Terminado cuando**

- no existen rutas conocidas de cross-tenant;
- borrar una fuente elimina derivados;
- los binarios se verifican;
- hay procedimiento de respuesta y divulgación.

### W22. Ecosistema de conectores y extensiones

**Construir**

- connector SDK;
- manifests;
- permisos declarados;
- lifecycle;
- firma;
- catálogo;
- compatibilidad;
- sandbox;
- documentación.

**Cómo**

- procesos separados;
- API limitada;
- credenciales mediadas;
- review;
- versionado;
- telemetry.

**Terminado cuando**

- un tercero crea un conector sin acceder a internals;
- el usuario ve permisos;
- un conector comprometido tiene alcance limitado;
- actualizar o retirar un conector no corrompe el proyecto.

### W23. Evaluación, simulación y calidad

**Construir**

- datasets;
- golden scenarios;
- simulador multiagente;
- fault injection;
- retrieval eval;
- context eval;
- model eval;
- performance benchmarks;
- security regression;
- product analytics.

**Cómo**

- snapshots reproducibles;
- carga sintética;
- fallos, duplicados y particiones;
- comparaciones exactas;
- shadow mode;
- métricas de utilidad real.

**Terminado cuando**

- cada cambio crítico se mide;
- los sistemas probabilísticos tienen baseline;
- una simulación reproduce carreras;
- las métricas muestran si Pact reduce coordinación manual.

---

## 26. Dependencias y orden de construcción

El mapa anterior es completo; este orden evita construir capas sobre conceptos inestables.

```text
W00 Especificación
  ↓
W01 Fundación ─────→ W02 Protocolo
  ↓                    ↓
W03 Datos y eventos ←──┘
  ↓
W04 Identidad
  ↓
W05 Delegación y políticas
  ↓
W06 Coordinación
  ├───────────────┐
  ↓               ↓
W07 Node        W10 Ingestión
  ↓               ↓
W08 Git         W11 Conocimiento
  ↓               ↓
W09 Código      W12 Retrieval
  └──────┬────────┘
         ↓
      W13 Contexto
         ↓
      W14 IA

W05 Políticas
  ↓
W15 Recursos → W16 IaC
  ↓             ↓
W17 Runners ←───┘
  ↓
W18 Secret broker

Todo lo anterior
  ↓
W19 UX/SDK → W20 Operación
  ↓             ↓
W21 Seguridad / W22 Ecosistema / W23 Evaluación
```

Secuencia ejecutable recomendada:

1. Formalizar entidades, estados, invariantes y eventos.
2. Crear repositorio, CI, schemas y codegen.
3. Levantar Go + PostgreSQL + pgvector.
4. Implementar comandos, transacciones, eventos, outbox y streams.
5. Implementar proyectos, actores y sesiones.
6. Implementar delegaciones, capacidades y denegación por defecto.
7. Implementar intenciones, scopes, leases y handoff.
8. Construir Pact Node y conexión local.
9. Crear worktrees y ChangeSets.
10. Integrar Git local y un proveedor remoto.
11. Añadir validaciones e integration queue.
12. Construir object storage y source registry.
13. Ingerir Git y documentos.
14. Implementar evidencia, decisiones y grafo temporal.
15. Activar full-text search y pgvector.
16. Construir recuperación híbrida y context packets.
17. Incorporar análisis de código incremental.
18. Añadir workers de IA con salidas estructuradas.
19. Construir ambientes y catálogo de recursos.
20. Implementar runners aislados.
21. Añadir Terraform/OpenTofu plan.
22. Incorporar políticas, aprobaciones y apply.
23. Integrar secret manager y credenciales efímeras.
24. Construir UI completa, SDKs y adaptadores.
25. Añadir conectores documentales y operativos.
26. Endurecer despliegue, HA, backup y restore.
27. Expandir simulación, seguridad y evaluación.
28. Añadir scale-outs solo donde las mediciones lo requieran.

En cada punto, las interfaces de las capacidades posteriores deben estar previstas, pero no es necesario simular que ya existen.

---

## 27. Estrategia de pruebas

### 27.1 Capas

#### Unitarias

- estados;
- políticas;
- normalizadores;
- parsers;
- ranking;
- redacción;
- idempotencia;
- validación de schemas.

#### Integración

- PostgreSQL real;
- pgvector;
- Git real;
- object storage;
- OIDC de prueba;
- secret provider falso y real controlado;
- Terraform en infraestructura desechable.

#### Contrato

- OpenAPI;
- eventos;
- SDKs;
- conectores;
- runners;
- versiones anteriores.

#### Concurrencia

- comandos simultáneos;
- leases;
- heartbeats;
- integration queue;
- workers;
- outbox;
- revocaciones.

#### End-to-end

- usuario;
- equipo;
- Git;
- contexto;
- infraestructura;
- recuperación.

#### Seguridad

- autorización;
- cross-tenant;
- prompt injection;
- command injection;
- SSRF;
- replay;
- token theft;
- revocación;
- logs;
- secretos;
- sandbox escape tests.

#### Recuperación y caos

- caída del servidor;
- caída de PostgreSQL;
- corte de red;
- evento duplicado;
- orden alterado;
- worker detenido;
- runner desaparecido;
- Git cambiado externamente;
- outcome unknown.

#### Evaluación probabilística

- extracción;
- entidades;
- relaciones;
- retrieval;
- contexto;
- citas;
- conflictos;
- coste.

### 27.2 Pruebas críticas obligatorias

- Dos comandos con la misma clave crean un solo efecto.
- Dos leases exclusivas simultáneas conceden una sola.
- Un fencing token antiguo no ejecuta.
- Una sesión expirada no renueva capacidad.
- Una intención sobrevive al agente.
- Un workspace puede transferirse.
- Dos integraciones concurrentes no pierden cambios.
- Una validación no se reutiliza tras cambiar el ChangeSet.
- Un push externo invalida contexto.
- Un cliente recupera eventos desde cursor.
- Una partición no concede autoridades globales contradictorias.
- Una inferencia no aparece como hecho observado.
- El modo estricto rechaza integración sin política.
- Un agente no escribe fuera de su workspace.
- Un documento malicioso no concede permisos.
- Un embedding no filtra contenido restringido.
- Borrar una fuente elimina sus derivados.
- Un plan modificado invalida aprobación.
- Un runner de desarrollo no ejecuta producción.
- Una credencial no aparece en logs.
- Una operación con conexión perdida pasa a `outcome_unknown`.
- Una restauración recupera estado y evidencia.

### 27.3 Simulador multiagente

Debe generar:

- cientos de sesiones;
- heartbeats;
- caídas;
- intenciones relacionadas;
- scopes cambiantes;
- eventos duplicados;
- cambios externos;
- carreras Git;
- conflictos;
- revocaciones;
- context requests;
- acciones y aprobaciones.

El simulador será una herramienta de diseño, no solo de carga.

---

## 28. Escenarios de aceptación integral

### 28.1 Una persona, dos agentes

**Dado**

- un repositorio;
- Pact local con PostgreSQL;
- dos chats del mismo usuario.

**Cuando**

- cada chat inicia una sesión;
- uno modifica autenticación;
- otro añade un endpoint relacionado.

**Entonces**

- tienen identidades diferentes y el mismo patrocinador;
- trabajan en worktrees separados;
- conocen las intenciones activas;
- Pact detecta solapamiento de contrato;
- ninguno necesita leer el chat del otro;
- los ChangeSets conservan atribución;
- la integración no pierde cambios.

### 28.2 Equipo de cuatro personas

**Dado**

- Pact Server compartido;
- cuatro Pact Nodes;
- repositorio remoto protegido.

**Cuando**

- varias personas y agentes trabajan a la vez;
- una intención queda bloqueada;
- otra depende de ella;
- un agente se desconecta.

**Entonces**

- el estado se actualiza en tiempo real;
- la dependencia refleja el bloqueo;
- la sesión expira;
- sus leases se liberan;
- la intención y workspace sobreviven;
- otra persona puede aceptar un handoff;
- toda acción conserva responsable.

### 28.3 Cambio directo en Git

**Dado**

- agentes trabajando sobre una revisión;
- una persona realiza push sin Pact.

**Cuando**

- llega el webhook.

**Entonces**

- Pact registra un cambio externo;
- no inventa una intención;
- actualiza la revisión canónica;
- reindexa;
- invalida contextos;
- marca ChangeSets afectados;
- notifica a sus agentes.

### 28.4 Reunión de cliente

**Dado**

- una transcripción con ACL restringida;
- participantes y consentimiento;
- una solicitud ambigua y una decisión explícita.

**Cuando**

- Pact ingiere la reunión.

**Entonces**

- conserva audio/transcripción y anclas;
- propone la solicitud como requisito candidato;
- registra la decisión con evidencia;
- no convierte la frase ambigua en decisión;
- un actor sin permiso no encuentra el contenido;
- un actor autorizado puede preparar la siguiente reunión.

### 28.5 Cambio en staging

**Dado**

- un agente con permiso de `infra.plan`;
- un runner de staging;
- Terraform;
- credenciales dinámicas.

**Cuando**

- propone cambiar capacidad de un servicio.

**Entonces**

- Pact genera plan;
- calcula recursos y riesgo;
- solicita aprobación si la política lo exige;
- vincula aprobación al hash;
- el runner obtiene credencial temporal;
- ejecuta exactamente el plan;
- comprueba resultado;
- no revela el secreto;
- actualiza topología y auditoría.

### 28.6 Cambio crítico en producción

**Dado**

- una operación R4;
- un agente patrocinado por un desarrollador;
- política de separación de funciones.

**Cuando**

- el agente solicita modificar IAM.

**Entonces**

- no puede autoaprobar;
- se exige MFA reciente y aprobadores autorizados;
- se utiliza runner de producción;
- la capacidad es de un solo uso;
- cualquier cambio en el plan invalida aprobación;
- todo queda auditado.

### 28.7 Pérdida de conexión durante una acción

**Dado**

- un runner aplicando una operación;
- conexión interrumpida después de enviar la solicitud.

**Cuando**

- Pact no conoce el resultado.

**Entonces**

- marca `outcome_unknown`;
- no reintenta automáticamente;
- consulta el proveedor;
- reconstruye estado;
- clasifica éxito, fallo o necesidad de intervención;
- conserva evidencia.

### 28.8 Recuperación completa

**Dado**

- pérdida del servidor;
- backups válidos;
- Git y fuentes externas disponibles.

**Cuando**

- se restaura PostgreSQL, objetos y configuración.

**Entonces**

- proyectos, identidades, eventos y políticas reaparecen;
- índices derivados pueden regenerarse;
- Git se reconcilia;
- leases antiguas no se reactivan;
- contextos expirados no se presentan como vigentes;
- auditoría verifica continuidad.

---

## 29. Métricas

### 29.1 Valor del producto

- tiempo hasta recibir contexto útil;
- tiempo de incorporación;
- explicaciones manuales evitadas;
- porcentaje de trabajo con intención;
- porcentaje de cambios con evidencia;
- conflictos detectados antes de integrar;
- retrabajo;
- tiempo de handoff;
- cambios externos;
- tasa de adopción de modo administrado;
- satisfacción y confianza.

La métrica principal no será el número de eventos almacenados, sino la reducción de coordinación manual y errores por contexto fragmentado.

### 29.2 Coordinación

- agentes simultáneos;
- scopes activos;
- solapamientos;
- falsos positivos;
- leases expiradas;
- wait time;
- sesiones huérfanas;
- handoffs;
- rebase rate;
- integration queue latency;
- intención a revisión aceptada.

### 29.3 Conocimiento

- cobertura de fuentes;
- ingest lag;
- parsing success;
- estabilidad de anclas;
- duplicados;
- conocimiento propuesto/aceptado/disputado;
- conocimiento obsoleto;
- contradicciones;
- precisión temporal;
- cobertura de evidencia.

### 29.4 Retrieval y contexto

- Recall@K;
- MRR;
- nDCG;
- recall ANN frente a exacto;
- latencia p50/p95/p99;
- diversidad;
- groundedness;
- precisión de citas;
- restricciones críticas incluidas;
- tokens entregados y utilizados;
- feedback.

### 29.5 Infraestructura y seguridad

- acciones por riesgo;
- denegaciones;
- aprobaciones;
- tiempo de aprobación;
- credenciales emitidas y TTL;
- revocaciones;
- drift;
- outcome unknown;
- break-glass;
- exposición de secretos;
- intentos cross-tenant;
- coste.

---

## 30. Riesgos de producto y arquitectura

| Riesgo | Consecuencia | Respuesta |
|---|---|---|
| Alcance excesivo | Nunca entregar una parte útil | Mantener primitivas comunes y construir por dependencias |
| Demasiada fricción | Usuarios evitan Pact | Inferencia, defaults, modo observador y automatización |
| Contexto obsoleto convincente | Decisiones incorrectas | Vigencia, revisión, lag visible e invalidación |
| Falsos conflictos | Ruido y pérdida de confianza | Confianza, evidencia, feedback y bloqueo solo por reglas fuertes |
| Conflictos no detectados | Integraciones rotas | Pruebas y validación continúan siendo autoridad |
| Ontología enorme | Modelo imposible de mantener | Vocabulario pequeño y extensible |
| IA central como cuello de botella | Coste, latencia y fragilidad | IA reemplazable; estado fuera del modelo |
| Control plane comprometido | Acceso amplio | Credenciales efímeras y runners/ambientes separados |
| Secretos en conocimiento | Fuga grave | Referencias, no valores; escaneo y no indexación |
| RLS mal configurado | Cruce de tenants | Autorización de aplicación y tests como capas adicionales |
| Bypass de Pact | Estado incompleto | Webhooks, reconciliación y ramas protegidas |
| Crecimiento de PostgreSQL | Latencia y coste | Partición, archivo y proyecciones externas medidas |
| HNSW con filtros pobres | Resultados faltantes | Evaluar recall, iterative scan, partición y exact search |
| Coste de embeddings | Factura y reindexación | Hash, lotes, deduplicación y políticas |
| Dependencia de un proveedor | Bloqueo | Interfaces, exportación y modelos versionados |
| Plugins inseguros | Exfiltración | Firma, sandbox, permisos y revisión |
| Docker como requisito local | Fricción | Instalador guiado y modalidad administrada posterior |
| Plan/apply diferente | Cambio no aprobado | Hash exacto, regeneración y locking |
| Resultado desconocido | Doble ejecución | Reconciliación previa |
| Documentos maliciosos | Prompt injection | Contenido no confiable y autorización determinista |
| Privacidad de reuniones | Riesgo legal y de confianza | Consentimiento, clasificación, retención y borrado |
| Pact intenta sustituir herramientas maduras | Complejidad y riesgo | Integrar sistemas de autoridad |

---

## 31. Decisiones adoptadas

Estas decisiones forman la hipótesis de arquitectura actual y deben registrarse como ADRs cuando comience el código.

1. Pact será un plano de control, no un reemplazo universal.
2. Git seguirá siendo autoridad sobre el código.
3. PostgreSQL será la base canónica desde el modo personal.
4. pgvector formará parte de la arquitectura completa.
5. Go implementará el núcleo, node y runner.
6. El despliegue personal utilizará PostgreSQL, probablemente mediante Docker.
7. El núcleo será un monolito modular.
8. Los eventos serán duraderos y el estado tendrá proyecciones relacionales.
9. Las conversaciones no serán la memoria canónica.
10. El contexto se compilará por intención y permisos.
11. Pact no almacenará secretos de usuario en texto claro.
12. Las credenciales serán efímeras y mediadas por runners.
13. La IA no será autoridad central.
14. Las acciones sensibles serán tipadas, autorizadas y auditadas.
15. Los worktrees proporcionarán aislamiento inicial.
16. Pact tolerará cambios externos.
17. El protocolo será independiente de proveedores.
18. La infraestructura declarativa seguirá en Git.
19. El estado IaC vivirá en backend protegido.
20. La recuperación híbrida combinará filtros, texto, grafo y vectores.

---

## 32. Decisiones abiertas

No bloquean la visión, pero deben resolverse antes del componente correspondiente.

### Producto

- ¿Qué parte será open source?
- ¿Cuál será el límite entre producto administrado y autohospedado?
- ¿El primer usuario objetivo será individual, equipo pequeño o plataforma interna?
- ¿Qué escenario demostrará primero valor diferencial?

### Protocolo

- ¿Adoptar CloudEvents literalmente o un perfil compatible?
- ¿SSE, WebSocket o ambos como transporte público inicial?
- ¿Cómo expresar `ResourceRef` de manera extensible?
- ¿Qué garantías de orden se ofrecen por proyecto?

### Identidad

- ¿Proveedor OIDC embebido o integración externa?
- ¿Tokens opacos o JWT para capacidades?
- ¿Cuándo incorporar SPIFFE?
- ¿Cómo registrar modelos/agentes sin confundir modelo, instancia y sesión?

### Datos

- ¿Object storage local basado en filesystem o servicio compatible?
- ¿Qué estrategia de partición usar al crecer?
- ¿Qué eventos se conservan indefinidamente?
- ¿Qué partes de auditoría necesitan almacenamiento WORM?

### Conocimiento

- ¿Cuál será la ontología mínima?
- ¿Qué fuentes se conectarán primero?
- ¿Qué decisiones requieren aprobación humana?
- ¿Qué modelo de embeddings y qué dimensión se usarán inicialmente?
- ¿Qué corpus medirá retrieval?

### Git y código

- ¿Qué proveedor remoto será el primer adaptador?
- ¿Qué lenguajes serán soportados primero?
- ¿Cuándo un scope inferido puede exigir revisión?
- ¿Qué estrategia de integración será predeterminada?

### Infraestructura

- ¿Terraform, OpenTofu o ambos desde el primer adaptador?
- ¿Qué backend de estado se recomendará?
- ¿Qué sandbox se usará para runners?
- ¿Qué acciones se permitirán en producción inicialmente?
- ¿Qué gestor de secretos será la referencia?

### Privacidad y operación

- ¿Dónde pueden residir datos y embeddings?
- ¿Cómo se gestiona consentimiento para reuniones?
- ¿Qué SLOs se prometerán?
- ¿Qué modalidad de actualización y soporte existirá?

Orden recomendado para resolverlas:

1. usuario y escenario inicial;
2. protocolo e identidades;
3. autoridad y seguridad;
4. Git y coordinación;
5. conocimiento y evaluación;
6. infraestructura y ejecución;
7. escalado y comercialización.

---

## 33. Ejemplo de configuración de proyecto

```yaml
apiVersion: pact.dev/v1alpha1
kind: Project

metadata:
  name: pact
  projectId: prj_example

spec:
  governanceMode: managed

  repositories:
    - name: core
      provider: generic-git
      canonicalRef: refs/heads/main
      path: .

  environments:
    - name: local
      risk: R0
    - name: staging
      risk: R2
    - name: production
      risk: R4

  coordination:
    workspaceStrategy: git-worktree
    leaseDefaults:
      code: advisory
      infrastructure: exclusive

  knowledge:
    sources:
      - repository: core
    retrieval:
      fullText: true
      vector: true
      graph: true

  policies:
    paths:
      - pact/policies

  validations:
    required:
      - unit-tests
      - protocol-conformance

  infrastructure:
    adapters:
      - terraform

  security:
    defaultDecision: deny
    secrets:
      revealToAgents: false
```

Este archivo no contiene tokens, contraseñas, IDs de lease ni estado operativo.

---

## 34. Criterio de producto completo

Pact habrá materializado esta visión cuando:

- una persona pueda instalarlo y conectar varios agentes;
- un equipo pueda compartir estado sin compartir chats;
- toda acción tenga identidad, intención y evidencia;
- los agentes trabajen de forma aislada y se integren de manera segura;
- Git directo pueda reconciliarse;
- documentos, reuniones, código e infraestructura formen una memoria citable;
- el contexto sea específico, autorizado, vigente y reproducible;
- pgvector mejore recuperación sin convertirse en autoridad;
- una IA pueda razonar sobre la infraestructura sin recibir claves permanentes;
- los runners ejecuten capacidades temporales y tipadas;
- los ambientes y permisos estén separados;
- los cambios sensibles requieran políticas y aprobaciones;
- el sistema sobreviva a desconexiones, duplicados y fallos;
- la base y los objetos puedan restaurarse;
- las fuentes y derivados puedan exportarse o eliminarse;
- las evaluaciones demuestren utilidad, calidad y aislamiento;
- sea posible sustituir modelos y proveedores sin rediseñar el protocolo.

La visión final puede resumirse así:

```text
Pact Server = autoridad del estado compartido
Pact Node   = observador y ejecutor local
Pact Runner = frontera segura de ejecución
Git         = autoridad del código
PostgreSQL  = autoridad del modelo operativo y conocimiento estructurado
Object store = evidencia y artefactos grandes
pgvector    = recuperación semántica derivada
Secret manager = custodia de credenciales
Intent      = unidad causal de trabajo
Capability  = autoridad temporal y limitada
Event       = hecho durable
Context     = vista autorizada y reproducible
IA          = participante y asesora, no autoridad implícita
```

> Pact hace que un proyecto sea comprensible y operable como un sistema vivo sin renunciar al aislamiento, la trazabilidad, la seguridad ni las fuentes originales de verdad.

---

## 35. Referencias técnicas base

- [PostgreSQL: control de concurrencia](https://www.postgresql.org/docs/current/mvcc-intro.html)
- [PostgreSQL: Row-Level Security](https://www.postgresql.org/docs/current/ddl-rowsecurity.html)
- [PostgreSQL: `LISTEN` y `NOTIFY`](https://www.postgresql.org/docs/current/sql-notify.html)
- [PostgreSQL: búsqueda textual](https://www.postgresql.org/docs/current/functions-textsearch.html)
- [pgvector: búsqueda vectorial e híbrida](https://github.com/pgvector/pgvector)
- [Git: worktrees](https://git-scm.com/docs/git-worktree.html)
- [Terraform: estado](https://developer.hashicorp.com/terraform/language/state)
- [Terraform: datos sensibles](https://developer.hashicorp.com/terraform/language/manage-sensitive-data)
- [Vault: secretos dinámicos para bases de datos](https://developer.hashicorp.com/vault/docs/secrets/databases)
- [Vault: leases, renovación y revocación](https://developer.hashicorp.com/vault/docs/concepts/lease)
- [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0-errata2.html)
- [OAuth 2.0 Token Exchange, RFC 8693](https://datatracker.ietf.org/doc/rfc8693/)
- [OAuth 2.0 Security Best Current Practice, RFC 9700](https://datatracker.ietf.org/doc/rfc9700/)
- [SPIFFE: identidad de workloads](https://spiffe.io/docs/latest/spiffe/concepts/)
- [Open Policy Agent: API](https://www.openpolicyagent.org/docs/rest-api)
- [CloudEvents](https://github.com/cloudevents/spec)
- [OpenTelemetry](https://opentelemetry.io/docs/)
