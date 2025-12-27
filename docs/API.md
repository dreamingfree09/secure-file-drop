# API Reference

This page documents the primary HTTP endpoints used by Secure File Drop. All endpoints are served on the server address (default `:8080`).

Authentication: /login returns a session cookie used for subsequent requests (cookie name `sfd_session` by default).

## POST /login
- Body: JSON {"username":"admin","password":"password"}
- Response: 200 {"status":"ok"}
- Side effect: sets a session cookie `sfd_session`

## POST /files
- Auth required
- Body: JSON {"orig_name": "file.txt", "content_type": "text/plain", "size_bytes": 123}
- Response: 201 {"id": "<uuid>", "object_key":"uploads/<uuid>", "status":"pending"}

## POST /upload?id=<uuid>
- Auth required
- Content-Type: multipart/form-data; field name `file`
- Response: 200 {"id": "<uuid>", "object_key":"uploads/<uuid>", "status":"hashed"}
- Errors: 413 file too large (when upload limit exceeded)

## POST /links
- Auth required
- Body: JSON {"id": "<uuid>", "ttl_seconds": 300}
- Response: 200 {"url": "https://host/download?token=<token>", "expires_at":"RFC3339 timestamp"}
- Error codes: 409 invalid status, 404 not found

## GET /download?token=<token>
- No auth required; token must be valid and unexpired
- Response: 200 with file content and headers:
  - Content-Type
  - Content-Length (when available)
  - Content-Disposition attachment; filename="<orig_name>"
- Error codes: 410 token expired, 401 invalid token

## Misc
- GET /health returns {"status":"ok"}
- GET /ready returns {"status":"ok"} when DB is reachable
- GET /version returns build information

For example `curl` usages, see `docs/USAGE.md`.