# Marca PACT — corchetes

## Archivos
- `pact-mark.svg` — marca sola sobre fondo claro (corchetes tinta, punto acento).
- `pact-mark-inverso.svg` — sobre fondo tinta.
- `pact-mark-mono-tinta.svg` / `pact-mark-mono-blanco.svg` — un solo color, para grabado, fax, sellos, fondos de color.
- `pact-lockup*.svg` — marca + wordmark PACT en JetBrains Mono 700, tracking 0.16em.
- `pact-icono-app.svg` + PNG 1024/512/256/180/64 — icono de aplicación (cuadrado tinta con radio 22%).
- `favicon.svg`, `favicon.ico` (16/32/48), `favicon-16.png`, `favicon-32.png`, `favicon-48.png`, `apple-touch-icon.png` (180).

## Construcción
Dos corchetes de trazo 5 (sobre caja de 48×40) y un punto de radio 5 centrado. El punto lleva el color del Workspace activo; los corchetes son siempre tinta o blanco, nunca de color.

## Reglas
- Aire mínimo alrededor: el ancho de un corchete (8 px en la caja base).
- Tamaño mínimo de la marca sola: 16 px de alto. Del lockup: 88 px de ancho.
- Nunca cambiar la proporción del punto ni separar los corchetes.
- No rotar, no aplicar sombra, no degradados, no contornear el wordmark.
- Sobre fotografía o color saturado, usar la versión monocroma blanca.

## HTML
```html
<link rel="icon" href="/favicon.ico" sizes="16x16 32x32 48x48">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
```
