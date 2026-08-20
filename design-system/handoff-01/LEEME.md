# PACT Control · handoff de diseño

Paquete de entrega del rediseño de PACT Control. Toda la interfaz está en español neutro.

## Empieza aquí
1. **`Handoff.html`** — explicación del diseño: modelo conceptual, navegación, principios, pantallas, sistema de diseño, marca, accesibilidad y decisiones abiertas. Se abre en cualquier navegador y se imprime a PDF.
2. **`pantallas/PACT Control v2.dc.html`** — prototipo navegable con las pantallas finales.

## Cómo abrir las pantallas
Los archivos `.dc.html` son páginas autónomas: ábrelas con doble clic. Deben quedar en la misma carpeta que `support.js` (ya viene incluido en `pantallas/`).

| Archivo | Contenido |
| --- | --- |
| `PACT Control v2.dc.html` | Prototipo navegable: Resumen, Trabajo en vivo con drawer de intent, colisión de scope, agente stale, Conversaciones, Contexto, Repositorios y detalle, Personas y agentes, Acceso, Actividad, Configuración, explorador de Workspaces y selector de color por Workspace. |
| `PACT Pantallas auxiliares.dc.html` | Login, autorización de dispositivo, invitación creada, GitHub no conectado, sala vacía, reconexión, sin permiso, carga, sin resultados, servidor caído, confirmación destructiva. |
| `PACT Design System.dc.html` | Tokens de color, tipografía, espacio y radios; botones, campos, chips de estado, identidad y presencia, tabla, banners, drawer, menús, toasts; comportamiento responsive. |
| `PACT Paletas y logo.dc.html` | Diez paletas y diez marcas exploradas, con sus conflictos semánticos anotados. |
| `PACT Exploraciones iniciales.dc.html` | Primeras tres direcciones visuales del shell. |

## Marca
`marca/` trae la marca de corchetes en SVG (color, inversa, monocromas y lockups), PNG (icono de app 1024/512/256/64, apple-touch-icon 180, favicons 48/32/16, marca suelta transparente), `favicon.svg` y `favicon.ico` multi-tamaño. Reglas de uso en `marca/USO.md`.

```html
<link rel="icon" href="/favicon.ico" sizes="16x16 32x32 48x48">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
```

## Dos reglas que no se negocian
- El color de marca lo elige cada Workspace y **nunca** comunica estado.
- El estado siempre combina color con palabra e icono o forma.
