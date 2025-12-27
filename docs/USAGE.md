# Usage

This document provides step-by-step examples for common workflows: logging in, creating a file record, uploading content, creating a download link, and downloading a file.

> Note: the server expects authentication via a session cookie set by POST /login. All write actions require authentication.

## Environment variables (examples)

- SFD_ADMIN_USER (e.g. `admin`)
- SFD_ADMIN_PASS (strong password)
- SFD_SESSION_SECRET (random string for HMAC-signed sessions)
- SFD_DOWNLOAD_SECRET (random string for signing download tokens)
- SFD_MINIO_ENDPOINT, SFD_MINIO_ACCESS_KEY, SFD_MINIO_SECRET_KEY, SFD_MINIO_BUCKET
- SFD_DB_DSN (Postgres connection string)
- SFD_PUBLIC_BASE_URL (optional; used to generate deterministic download links)

## Login

Request:

curl -v -X POST -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"password"}' \
  http://localhost:8080/login -c cookies.txt

- The `-c cookies.txt` flag saves the session cookie for subsequent requests.

## Create file metadata

Request:

curl -v -X POST -H "Content-Type: application/json" \
  -d '{"orig_name":"example.txt","content_type":"text/plain","size_bytes":123}' \
  http://localhost:8080/files -b cookies.txt

Response (201):

{
  "id": "<uuid>",
  "object_key": "uploads/<uuid>",
  "status": "pending"
}

Save the returned `id` for the upload step.

## Upload file

Request (multipart form):

curl -v -X POST -F "file=@./example.txt" \
  "http://localhost:8080/upload?id=<uuid>" -b cookies.txt

Notes:
- The multipart field name must be `file`.
- If `SFD_MAX_UPLOAD_BYTES` is configured, uploads larger than that will be rejected with 413.

Response (200):

{
  "id": "<uuid>",
  "object_key": "uploads/<uuid>",
  "status": "hashed"
}

## Create a signed download link

Request:

curl -v -X POST -H "Content-Type: application/json" \
  -d '{"id":"<uuid>","ttl_seconds":300}' \
  http://localhost:8080/links -b cookies.txt

Response (200):

{
  "url": "https://your-host/download?token=<signed-token>",
  "expires_at": "2025-12-27T12:34:56Z"
}

## Download

GET the provided URL (no authentication required if token is valid):

curl -v "https://your-host/download?token=<signed-token>" -O

## Troubleshooting

- Check `/health` and `/ready` for service status.
- Inspect `journal/DEVLOG.md` for development notes and known issues.

For API request/response examples and details, see `docs/API.md`.