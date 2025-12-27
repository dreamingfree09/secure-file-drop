# Frontend / Static site notes

The frontend is intentionally minimal and lives in `web/static/index.html`. The server mounts the `web/static/` directory at `/static/` and serves the index at `/`.

## Current behavior

- A tiny single-page UI supports:
  - Login (POST /login)
  - Upload flow: create metadata (/files) -> upload multipart to /upload?id=<id> -> request link (/links)
  - Displaying the returned signed link
- The UI relies on same-origin requests and session cookie authentication.

## How to extend

- Add CSS/JS assets into `web/static/` and reference them from `index.html`.
- For larger frontends, consider adding a build step that outputs to `web/static/` (for example a small React/Vue app built into the `web/static` folder).
- Keep authentication via the session cookie and avoid exposing the download-secret to the client.

## Deployment

- Static files are served by the backend process in the container (see `New` in `internal/server/server.go`). In production, a reverse proxy can serve static assets directly for performance.

If you'd like, I can convert the UI into a slightly larger front-end scaffold (with a build pipeline) and add a small set of end-to-end tests for the upload flow.