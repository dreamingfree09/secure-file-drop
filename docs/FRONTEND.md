# Frontend / Static site notes

The frontend features a modern, WeTransfer-inspired design and lives in `web/static/index.html`. The server mounts the `web/static/` directory at `/static/` and serves the index at `/`.

## Current behavior

- Modern single-page UI with:
  - Animated gradient background (purple/pink theme)
  - User registration with client-side validation (POST /register)
  - Login screen (POST /login)
  - Drag-and-drop file upload with visual feedback
  - **Real-time upload progress tracking** with XMLHttpRequest
    - Shows exact bytes transferred (e.g., "Uploading... 245.3MB / 512.0MB (48%)")
    - Dynamic progress bar updates during upload
    - Supports files up to 50GB (configurable via SFD_MAX_UPLOAD_BYTES)
  - Progress bars with shimmer animations
  - One-click copy-to-clipboard for download links
  - Responsive mobile design
  - Admin dashboard with metrics and file management
  - My Uploads controls:
    - Sorting (newest, oldest, name, size)
    - Search by filename
    - Status filter (pending/stored/hashed/failed)
    - Compact view toggle
    - Collapsible section (default collapsed; persisted via localStorage)
    - Batch delete with selection, select-all, clear selection
    - Select "ready" files only
    - Keyboard shortcuts for common actions
  - Session persistence on refresh via `/me`
  - Logout endpoint wired to `/logout`
- The UI relies on same-origin requests and session cookie authentication.
- Uses Inter font from Google Fonts for professional typography
- CSS custom properties for consistent theming
- Native browser download progress (Content-Length headers enable browser's download UI)

## How to extend

- Add CSS/JS assets into `web/static/` and reference them from `index.html`.
- For larger frontends, consider adding a build step that outputs to `web/static/` (for example a small React/Vue app built into the `web/static` folder).
- Keep authentication via the session cookie and avoid exposing the download-secret to the client.

## Deployment

- Static files are served by the backend process in the container (see `New` in `internal/server/server.go`). In production, a reverse proxy can serve static assets directly for performance.

## Notes

- Public download links use `SFD_PUBLIC_BASE_URL` when available.
- Upload progress shows true 0–100% and stops shimmer when complete.
- The UI enforces duplicate prevention with a queue and in-progress flag.

If you'd like, I can convert the UI into a slightly larger front-end scaffold (with a build pipeline) and add a small set of end-to-end tests for the upload flow.