# ADR-0010 — Cliente nativo para Windows

**Estado:** aceptado  
**Fecha:** 14 de agosto de 2026

## Contexto

Un equipo no puede asumir que todos sus participantes utilizan macOS, Linux o
WSL. Un colaborador que abre el repositorio desde Codex, VS Code, Claude u otro
cliente en Windows necesita exactamente el mismo vínculo con Pact Server, la
misma coordinación y los mismos worktrees que el resto del equipo.

Compilar un archivo `.exe` no basta. Windows utiliza otra ubicación para la
configuración privada, no implementa las señales POSIX del mismo modo, conserva
permisos mediante ACL y puede limitar rutas extensas de Git. La instalación
tampoco debe exigir clonar el repositorio ni instalar Go.

## Decisión

Pact ofrece un cliente nativo para Windows 10 y 11 en `amd64` y `arm64`:

- cada versión publica `pact_windows_amd64.zip` y
  `pact_windows_arm64.zip`;
- `install-pact.ps1` detecta la arquitectura, descarga el artefacto de GitHub,
  valida su SHA-256, instala en `%LOCALAPPDATA%\Programs\Pact` y actualiza el
  `PATH` del usuario;
- si el repositorio es privado, el instalador reutiliza `gh auth token` o acepta
  `GH_TOKEN`, `GITHUB_TOKEN` y `-GitHubToken`; descarga los artefactos mediante
  la API autenticada sin persistir esa credencial;
- la credencial personal se guarda en `%APPDATA%\Pact\config.json`, fuera de
  los repositorios y bajo las ACL del perfil del usuario;
- `.pact/`, `pact.yaml` y `.codex/config.toml` conservan el mismo significado
  en los tres sistemas operativos;
- al cargar un proyecto conectado, Pact habilita `core.longpaths=true` solo en
  la configuración local de ese repositorio;
- el cierre usa `SIGINT` y `SIGTERM` en Unix. En Windows escucha la interrupción
  de consola y termina de forma explícita el proceso hijo, ya que Go no puede
  enviar `os.Interrupt` de forma fiable a un proceso arbitrario;
- el CI ejecuta `go vet` y todas las pruebas en Windows, macOS y Linux, además
  de compilar ambos artefactos de Windows en cada cambio a `main`;
- una versión no queda validada hasta que un runner de Windows instala el
  artefacto publicado y ejecuta el CLI.

Git for Windows es una dependencia explícita. Pact no incorpora un cliente Git
alternativo ni ejecuta una base de datos local: el CLI se comunica por HTTPS con
el Pact Server compartido.

La incorporación al catálogo comunitario de WinGet queda condicionada a que
los ZIP tengan una URL pública estable. Los manifiestos se generan desde la
release desde ahora para que ese paso no requiera rediseñar el paquete.

## Consecuencias

- una persona puede clonar, instalar, iniciar sesión, ejecutar `pact connect` y
  habilitar el MCP de Codex enteramente desde PowerShell;
- los secretos y rutas locales no se añaden a Git;
- los worktrees administrados cuentan con soporte para rutas largas;
- las garantías Unix basadas en modos `0600` y `0700` se expresan en Windows
  mediante ACL del perfil, no mediante bits POSIX;
- WSL sigue siendo compatible como entorno Linux independiente, pero ya no es
  un requisito para utilizar Pact en Windows.
