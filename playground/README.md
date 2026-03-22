# Playground (standalone static UI)

This directory is a **pure static frontend**, separate from the Go Realtime server:

- `index.html` — markup
- `styles.css` — styles
- `app.js` — behavior

## Usage

1. Start the Realtime server (default `http://localhost:8080`) and enable CORS (`CORS_ALLOWED_ORIGINS`; dev may use `*`).
2. Serve this directory with any static file server, for example:
   - From the repo root with Docker Compose: `docker compose up` then open **http://localhost:3000**
   - Locally: `npx --yes serve . -l 3000` (run inside `playground/`)
3. On the page, set **Realtime backend** base URL and click **Generate WebSocket URL from backend**, then connect and send broadcasts.

## Relationship to Go

- This page is **not** served by a Go HTTP route; Go only exposes `/ws` and `/api/event`.
- When the browser loads the UI from another port (e.g. 3000), requests to 8080 are cross-origin; the Go server must send appropriate CORS headers.
