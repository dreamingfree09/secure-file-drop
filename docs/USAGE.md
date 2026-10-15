# Configuration and usage

Read [deployment](DEPLOYMENT.md) before starting a stack and [API](API.md) for request contracts. The application does not load .env by itself; Compose interpolates it. Verify connection targets before startup because migrations run automatically and cleanup is enabled by default.

| Variable | Meaning/default |
| --- | --- |
| DATABASE_URL | Required PostgreSQL DSN; startup applies embedded migrations |
| SFD_ADMIN_USER | Environment administrator name; default admin |
| SFD_ADMIN_PASS | Required administrator password |
| SFD_SESSION_SECRET | Required HMAC session secret, at least 32 bytes |
| SFD_DOWNLOAD_SECRET | Required for signed links, at least 32 bytes |
| SFD_S3_ENDPOINT | Required MinIO endpoint; http/https or host:port |
| SFD_S3_ACCESS_KEY / SFD_S3_SECRET_KEY | Required storage credentials |
| SFD_BUCKET | Required existing private bucket, with no bucket policy |
| SFD_MAX_UPLOAD_BYTES | Positive maximum file bytes; default 50 GiB; example config uses 10 MiB |
| SFD_PUBLIC_BASE_URL | Canonical public origin, e.g. https://files.example.test; configures secure cookies |
| SFD_ADDR | Listen address; default :8080 |
| SFD_HASH_TOOL | Native CLI path; default /app/sfd-hash |
| SFD_WEB_DIR | Static asset directory; default web/static |
| SFD_CLEANUP_ENABLED | true by default; false disables cleanup |
| SFD_CLEANUP_INTERVAL | Positive duration; default 1h |
| SFD_CLEANUP_MAX_AGE | Positive duration; default 24h |
| SFD_CLEANUP_MAX_AGE_HOURS | Positive integer compatibility override |
| SFD_VERSION / SFD_COMMIT | Build metadata returned by /version |

SFD_DB_DSN and SFD_MINIO_* are historical documentation names; the backend reads DATABASE_URL and SFD_S3_* / SFD_BUCKET. Compose maps its MinIO root variables into the backend storage variables for local development. Production should use independently managed credentials.

Register an ordinary account, log in, create metadata, upload multipart file bytes, then create a signed link. The browser implements this sequence. Ordinary users see their files; the administrator additionally sees global counters, deletion and manual cleanup. Sign out calls the server and revokes the current session. Signed download links remain bearer links until expiry/deletion, independent of logout.

Large-file behavior is bounded but this review does not certify a 50 GiB throughput or capacity target. Hashing requires temporary disk capacity for a complete object and has a 2-minute subprocess timeout within a 5-minute upload operation. Tune operational limits only after representative load tests. See [roadmap](ROADMAP.md) for future ideas.
