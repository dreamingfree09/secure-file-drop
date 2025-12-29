# Usage

This document provides step-by-step examples for common workflows: logging in, verifying session, creating a file record, uploading content, creating a download link, downloading a file, and logging out.

> Note: the server expects authentication via a session cookie set by POST /login. All write actions require authentication.

## Environment variables (examples)

- SFD_ADMIN_USER (e.g. `admin`)
- SFD_ADMIN_PASS (strong password)
- SFD_SESSION_SECRET (random string for HMAC-signed sessions)
- SFD_DOWNLOAD_SECRET (random string for signing download tokens)
- SFD_MAX_UPLOAD_BYTES (max upload size in bytes, default: 50GB = 53687091200)
- SFD_MINIO_ENDPOINT, SFD_MINIO_ACCESS_KEY, SFD_MINIO_SECRET_KEY, SFD_MINIO_BUCKET
- DATABASE_URL (Postgres connection string)
- SFD_PUBLIC_BASE_URL (preferred; used to generate public download links)

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
- Default upload limit is 50GB. Configure via `SFD_MAX_UPLOAD_BYTES` environment variable.
- Files larger than the limit will be rejected with HTTP 413.
- The web UI shows real-time upload progress with bytes transferred and percentage.

Response (200):

{
  "id": "<uuid>",
  "object_key": "uploads/<uuid>",
  "status": "hashed"
}

## Create a signed download link

Request:

curl -v -X POST -H "Content-Type: application/json" \
  -d '{"file_id":"<uuid>","expires_in_hours":24}' \
  http://localhost:8080/links -b cookies.txt

Response (200):

{
  "download_url": "https://your-host/download?token=<signed-token>",
  "expires_at": "2025-12-27T12:34:56Z"
}

## Download

GET the provided URL (no authentication required if token is valid):

curl -v "https://your-host/download?token=<signed-token>" -O

If the link was created with a password, include `&password=<password>` in the URL.
## Verify Session

Check if you are still authenticated:

curl -v http://localhost:8080/me -b cookies.txt

Response (200):

{
  "status": "ok",
  "username": "admin",
  "is_admin": true
}

If the session is invalid or expired, the response will be `401 Unauthorized`.

## Logout

End the session and clear the cookie:

curl -v -X POST http://localhost:8080/logout -b cookies.txt

Response (200):

{ "status": "ok" }

## Troubleshooting

- Check `/health` and `/ready` for service status.
- Inspect `journal/DEVLOG.md` for development notes and known issues.

For API request/response examples and details, see `docs/API.md`.