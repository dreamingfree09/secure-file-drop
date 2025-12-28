# Secure File Drop API Documentation

## Overview

Secure File Drop provides a RESTful API for secure file uploads, downloads, and management. All authenticated endpoints require a session cookie obtained via `/login`.

**Base URL**: `http://localhost:8080` (configure via `SFD_BASE_URL`)

**Authentication**: Session-based (cookie: `sfd_session`)

**Rate Limiting**: 100 requests per minute per IP address

---

## Table of Contents

1. [Authentication](#authentication)
2. [User Registration & Verification](#user-registration--verification)
3. [File Operations](#file-operations)
4. [Download Links](#download-links)
5. [Admin Operations](#admin-operations)
6. [System Endpoints](#system-endpoints)
7. [Error Responses](#error-responses)

---

## Authentication

### Login

**POST** `/login`

Authenticate and receive a session cookie.

**Request Body:**
```json
{
  "username": "admin",
  "password": "your-password"
}
```

**Response:** `200 OK`
```json
{
  "status": "ok"
}
```

**Errors:**
- `401 Unauthorized`: Invalid credentials
- `400 Bad Request`: Missing username or password

**Cookie Set:** `sfd_session` (HttpOnly, 12-hour expiry)

---

### Verify Session

**GET** `/me`

Check if current session is valid.

**Headers:**
```
Cookie: sfd_session=<token>
```

**Response:** `200 OK`
```json
{
  "status": "ok"
}
```

**Errors:**
- `401 Unauthorized`: Invalid or expired session

---

## User Registration & Verification

### Register User

**POST** `/register`

Create a new user account.

**Request Body:**
```json
{
  "email": "user@example.com",
  "username": "johndoe",
  "password": "SecurePass123!"
}
```

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "username": "johndoe"
}
```

**Errors:**
- `400 Bad Request`: Invalid input (email/username/password format)
- `409 Conflict`: Username or email already exists

**Side Effect:** Sends verification email to registered address

**Password Requirements:**
- Minimum 8 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one number

---

### Verify Email

**GET** `/verify?token={verification-token}`

Verify email address using token from email.

**Query Parameters:**
- `token` (required): Email verification token

**Response:** `200 OK`
```json
{
  "status": "verified",
  "message": "Email verified successfully"
}
```

**Errors:**
- `400 Bad Request`: Token missing
- `401 Unauthorized`: Invalid or expired token

---

### Request Password Reset

**POST** `/reset-password-request`

Request a password reset email.

**Request Body:**
```json
{
  "email": "user@example.com"
}
```

**Response:** `200 OK`
```json
{
  "status": "ok",
  "message": "If the email exists, a reset link has been sent"
}
```

**Note:** Always returns success to prevent email enumeration

**Side Effect:** Sends password reset email if address exists

---

### Reset Password

**POST** `/reset-password`

Complete password reset using token from email.

**Request Body:**
```json
{
  "token": "reset-token-from-email",
  "new_password": "NewSecurePass123!"
}
```

**Response:** `200 OK`
```json
{
  "status": "ok",
  "message": "Password reset successful"
}
```

**Errors:**
- `400 Bad Request`: Missing token or password
- `401 Unauthorized`: Invalid or expired token (1 hour expiry)
- `400 Bad Request`: Password doesn't meet requirements

---

## File Operations

### Get Configuration

**GET** `/config`

Get server configuration for client validation.

**Response:** `200 OK`
```json
{
  "max_upload_bytes": 53687091200,
  "version": "v1.0.0"
}
```

**Fields:**
- `max_upload_bytes`: Maximum file size (0 = unlimited)
- `version`: Server version

---

### Create File Record

**POST** `/files`

Create a file metadata record before upload.

**Headers:**
```
Cookie: sfd_session=<token>
Content-Type: application/json
```

**Request Body:**
```json
{
  "orig_name": "document.pdf",
  "content_type": "application/pdf",
  "size_bytes": 1048576,
  "ttl_hours": 24
}
```

**Fields:**
- `orig_name` (required): Original filename
- `content_type` (required): MIME type
- `size_bytes` (required): File size in bytes
- `ttl_hours` (optional): Auto-delete after N hours (0 = never)

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "object_key": "uploads/550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

**Errors:**
- `401 Unauthorized`: Not authenticated
- `400 Bad Request`: Invalid input (missing fields, negative size)
- `403 Forbidden`: Storage quota exceeded
- `500 Internal Server Error`: Database error

---

### Upload File Data

**POST** `/upload?id={file-id}`

Upload file content to storage.

**Headers:**
```
Cookie: sfd_session=<token>
Content-Type: multipart/form-data
```

**Query Parameters:**
- `id` (required): File ID from `/files` response

**Form Data:**
- `file`: Binary file content

**Response:** `200 OK`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "object_key": "uploads/550e8400-e29b-41d4-a716-446655440000",
  "status": "hashed"
}
```

**Status Progression:**
1. `pending` → File record created
2. `stored` → File uploaded to storage
3. `hashed` → SHA-256 hash computed, ready for download

**Errors:**
- `401 Unauthorized`: Not authenticated
- `400 Bad Request`: Missing file ID or file data
- `404 Not Found`: File ID doesn't exist
- `413 Payload Too Large`: File exceeds size limit
- `502 Bad Gateway`: Storage or hashing failure

**Side Effect:** Sends upload complete email when hashing finishes

---

### List User Files

**GET** `/user/files`

Get list of files uploaded by current user.

**Headers:**
```
Cookie: sfd_session=<token>
```

**Response:** `200 OK`
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "object_key": "uploads/550e8400-e29b-41d4-a716-446655440000",
    "orig_name": "document.pdf",
    "content_type": "application/pdf",
    "size_bytes": 1048576,
    "status": "hashed",
    "created_at": "2025-12-28T10:30:00Z",
    "sha256_hex": "abc123...",
    "expires_at": "2025-12-29T10:30:00Z",
    "auto_delete": true,
    "download_count": 5,
    "last_downloaded_at": "2025-12-28T15:45:00Z"
  }
]
```

**Errors:**
- `401 Unauthorized`: Not authenticated

---

### Get User Quota

**GET** `/quota`

Get current user's storage usage and quota.

**Headers:**
```
Cookie: sfd_session=<token>
```

**Response:** `200 OK`
```json
{
  "storage_used_bytes": 524288000,
  "storage_quota_bytes": 10737418240
}
```

**Fields:**
- `storage_used_bytes`: Current storage usage
- `storage_quota_bytes`: Total quota (0 = unlimited)

**Errors:**
- `401 Unauthorized`: Not authenticated

---

## Download Links

### Create Download Link

**POST** `/links`

Create a signed download link with optional password and expiration.

**Headers:**
```
Cookie: sfd_session=<token>
Content-Type: application/json
```

**Request Body:**
```json
{
  "file_id": "550e8400-e29b-41d4-a716-446655440000",
  "expires_in_hours": 24,
  "password": "optional-password"
}
```

**Fields:**
- `file_id` (required): ID of file to share
- `expires_in_hours` (optional): Link expiry (default: 24, max: 168)
- `password` (optional): Password protect the download

**Response:** `200 OK`
```json
{
  "download_url": "http://localhost:8080/download?token=eyJhbG...",
  "expires_at": "2025-12-29T10:30:00Z"
}
```

**Errors:**
- `401 Unauthorized`: Not authenticated
- `400 Bad Request`: Invalid file ID or expiry
- `404 Not Found`: File not found or not ready
- `403 Forbidden`: File not owned by current user

---

### Download File

**GET** `/download?token={signed-token}&password={password}`

Download a file using signed link.

**Query Parameters:**
- `token` (required): Signed download token from `/links`
- `password` (optional): Password if file is protected

**Response:** `200 OK`
```
Content-Type: <file-content-type>
Content-Disposition: attachment; filename="document.pdf"
Content-Length: 1048576

<binary file data>
```

**Errors:**
- `400 Bad Request`: Token missing
- `401 Unauthorized`: Invalid token or incorrect password
- `410 Gone`: Token expired or file deleted
- `404 Not Found`: File not found
- `502 Bad Gateway`: Storage retrieval failed

**Side Effect:** 
- Increments download counter
- Updates last downloaded timestamp
- Sends download notification email to file owner

---

## Admin Operations

### List All Files

**GET** `/admin/files`

Get list of all files in the system (admin only).

**Headers:**
```
Cookie: sfd_session=<token>
```

**Response:** `200 OK`
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "object_key": "uploads/550e8400-e29b-41d4-a716-446655440000",
    "orig_name": "document.pdf",
    "content_type": "application/pdf",
    "size_bytes": 1048576,
    "created_by": "johndoe",
    "status": "hashed",
    "created_at": "2025-12-28T10:30:00Z",
    "sha256_hex": "abc123...",
    "expires_at": "2025-12-29T10:30:00Z",
    "auto_delete": true,
    "download_count": 5,
    "last_downloaded_at": "2025-12-28T15:45:00Z"
  }
]
```

**Errors:**
- `401 Unauthorized`: Not authenticated or not admin

---

### Delete File

**DELETE** `/admin/files/{file-id}`

Delete a file from storage and database (admin only).

**Headers:**
```
Cookie: sfd_session=<token>
```

**Path Parameters:**
- `file-id`: UUID of file to delete

**Response:** `204 No Content`

**Errors:**
- `401 Unauthorized`: Not authenticated or not admin
- `404 Not Found`: File doesn't exist

**Side Effect:** Sends deletion notification email to file owner

---

### Get System Metrics

**GET** `/metrics`

Get system-wide usage statistics (admin only).

**Headers:**
```
Cookie: sfd_session=<token>
```

**Response:** `200 OK`
```json
{
  "uploads_total": 1250,
  "downloads_total": 3842,
  "storage_total_bytes": 524288000,
  "storage_total_files": 487,
  "login_success_total": 2891,
  "login_failures_total": 23,
  "files_ready_total": 450,
  "files_pending_total": 12,
  "files_failed_total": 25,
  "requests_total": 45623
}
```

**Errors:**
- `401 Unauthorized`: Not authenticated or not admin

---

### Manual Cleanup

**POST** `/admin/cleanup`

Trigger manual cleanup of expired/failed files (admin only).

**Headers:**
```
Cookie: sfd_session=<token>
```

**Response:** `200 OK`
```json
{
  "deleted_count": 15
}
```

**Errors:**
- `401 Unauthorized`: Not authenticated or not admin
- `503 Service Unavailable`: Cleanup disabled

**Cleanup Rules:**
- Deletes files in `pending` or `failed` status older than 1 hour
- Deletes files with `auto_delete=true` past `expires_at`
- Removes from both storage and database

---

## System Endpoints

### Health Check

**GET** `/health`

Basic liveness check (no authentication required).

**Response:** `200 OK`
```json
{
  "status": "ok"
}
```

---

### Readiness Check

**GET** `/ready`

Comprehensive dependency health check.

**Response:** `200 OK` (all healthy)
```json
{
  "status": "healthy",
  "dependencies": {
    "database": "healthy",
    "storage": "healthy"
  }
}
```

**Response:** `503 Service Unavailable` (degraded)
```json
{
  "status": "unhealthy",
  "dependencies": {
    "database": "unhealthy: connection refused",
    "storage": "healthy"
  }
}
```

---

### Version Info

**GET** `/version`

Get server version and build information.

**Response:** `200 OK`
```json
{
  "version": "v1.0.0",
  "commit": "abc123def"
}
```

---

## Error Responses

### Standard Error Format

All error responses follow this structure:

**Status:** `4xx` or `5xx`
**Body:** Plain text error message

```
invalid username or password
```

### Common HTTP Status Codes

| Code | Meaning | Common Causes |
|------|---------|---------------|
| `400` | Bad Request | Invalid input, missing required fields |
| `401` | Unauthorized | Invalid/expired session, wrong password |
| `403` | Forbidden | Quota exceeded, permission denied |
| `404` | Not Found | Resource doesn't exist |
| `405` | Method Not Allowed | Wrong HTTP method |
| `409` | Conflict | Username/email already exists |
| `410` | Gone | Token expired, file deleted |
| `413` | Payload Too Large | File exceeds size limit |
| `429` | Too Many Requests | Rate limit exceeded |
| `500` | Internal Server Error | Database/server error |
| `502` | Bad Gateway | Storage/hashing service failure |
| `503` | Service Unavailable | Service disabled or unhealthy |

---

## Rate Limiting

**Global Rate Limit:** 100 requests per minute per IP address

**Algorithm:** Token bucket (refills at constant rate)

**Response When Limited:** `429 Too Many Requests`

**Headers:**
```
Retry-After: 42
```

**Exempt Endpoints:** `/health`, `/ready`, `/version`

---

## File Status States

Files progress through these states:

1. **`pending`** → File record created, awaiting upload
2. **`stored`** → File uploaded to storage, awaiting hash
3. **`hashed`** → SHA-256 computed, ready for download
4. **`failed`** → Upload or hashing failed (auto-deleted after 1 hour)
5. **`ready`** → (Future) Admin-approved for download

Only files in `hashed` or `ready` status can be downloaded.

---

## File Size Limits

Configure via `SFD_MAX_UPLOAD_BYTES` environment variable:

```bash
# 50 GB limit
SFD_MAX_UPLOAD_BYTES=53687091200

# Unlimited
SFD_MAX_UPLOAD_BYTES=0
```

Clients should call `/config` to get the current limit before upload.

---

## Session Management

**Cookie Name:** `sfd_session`

**Attributes:**
- `HttpOnly`: Yes (prevents JavaScript access)
- `Secure`: Yes in production
- `SameSite`: Lax
- `Max-Age`: 12 hours

**Storage:** In-memory (cleared on server restart)

**Renewal:** Sessions do not auto-renew; re-login required after expiry

---

## Security Features

1. **HMAC-Signed Tokens**: Download links use HMAC-SHA256 for integrity
2. **Password Hashing**: Bcrypt with cost factor 10
3. **Session Secrets**: Require `SFD_SESSION_SECRET` environment variable
4. **Rate Limiting**: Global 100 req/min per IP
5. **Content-Type Validation**: Enforced on uploads
6. **File Quarantine**: Pending files auto-deleted after 1 hour
7. **Password Protection**: Optional per-file password on downloads
8. **Expiring Links**: Download tokens expire after 24 hours (default)

---

## Client Libraries

### JavaScript Example

```javascript
// Login
const login = async (username, password) => {
  const res = await fetch('/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
    credentials: 'include'
  });
  return res.ok;
};

// Create file and upload
const uploadFile = async (file) => {
  // 1. Create file record
  const metaRes = await fetch('/files', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({
      orig_name: file.name,
      content_type: file.type,
      size_bytes: file.size,
      ttl_hours: 24
    })
  });
  const { id } = await metaRes.json();

  // 2. Upload file data
  const formData = new FormData();
  formData.append('file', file);
  
  const uploadRes = await fetch(`/upload?id=${id}`, {
    method: 'POST',
    credentials: 'include',
    body: formData
  });
  
  return uploadRes.json();
};

// Create download link
const createLink = async (fileId) => {
  const res = await fetch('/links', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({
      file_id: fileId,
      expires_in_hours: 24
    })
  });
  const { download_url } = await res.json();
  return download_url;
};
```

### cURL Examples

```bash
# Login
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' \
  -c cookies.txt

# Create file record
curl -X POST http://localhost:8080/files \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"orig_name":"test.txt","content_type":"text/plain","size_bytes":1024}'

# Upload file
curl -X POST "http://localhost:8080/upload?id=<file-id>" \
  -b cookies.txt \
  -F "file=@test.txt"

# Create download link
curl -X POST http://localhost:8080/links \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"file_id":"<file-id>","expires_in_hours":24}'

# Download file
curl -O "http://localhost:8080/download?token=<signed-token>"
```

---

## Additional Resources

- **Email Configuration**: See [EMAIL_NOTIFICATIONS.md](EMAIL_NOTIFICATIONS.md)
- **Deployment Guide**: See [../README.md](../README.md)
- **Spec Document**: See [SPEC.md](SPEC.md)
- **Development Log**: See [../journal/DEVLOG.md](../journal/DEVLOG.md)

---

**Last Updated:** 2025-12-28
**API Version:** 1.0
**Server Version:** See `/version` endpoint
