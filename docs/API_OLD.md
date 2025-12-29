# API Reference (Deprecated)

This page is preserved for historical reference. Please refer to the up-to-date API documentation in [docs/API.md](API.md).

Authentication: /login returns a session cookie used for subsequent requests (cookie name `sfd_session` by default).

## POST /register
- Body: JSON {"email":"user@example.com","username":"myusername","password":"securepass123"}
- Response: 201 {"id":"<uuid>","email":"user@example.com","username":"myusername"}
- Validation:
  - Email must be valid format
  - Username: 3-50 characters, alphanumeric + underscore only
  - Password: minimum 8 characters, must contain both letters and numbers
- Errors: 400 validation failed, 409 email/username already exists

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
- Errors: 413 file too large (default limit: 50GB, configurable via SFD_MAX_UPLOAD_BYTES)
- Note: Upload progress is tracked client-side using XMLHttpRequest with progress events

## POST /links (see API.md for current spec)
Body shape and fields have been updated. Use `file_id` and `expires_in_hours`, and note that responses return `download_url`.

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

For example `curl` usages, see [docs/USAGE.md](USAGE.md) and [docs/API.md](API.md).