# Deployment

The checked-in Compose file is a local development configuration. Its published ports bind loopback; it uses persistent development volumes. Do not use it as a test harness or reuse existing volumes for validation. [The full-stack test](../tests/e2e/e2e_test.go) provisions its own disposable resources.

## Fresh local development only

1. Copy .env.example to .env and replace placeholders with independently generated strong passwords/secrets. Set a free, unique Compose project name with -p for the fresh stack. Do not start against an existing database/storage unintentionally.
2. Start only dependencies: docker compose -p YOUR_UNIQUE_PROJECT up -d --build postgres minio.
3. On that verified fresh MinIO, create the private bucket:

```sh
docker compose -p YOUR_UNIQUE_PROJECT exec minio sh -c 'mc alias set local http://localhost:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null && mc mb "local/$SFD_BUCKET"'
```

4. Start backend/proxy with the same project: docker compose -p YOUR_UNIQUE_PROJECT up -d --build backend proxy. Embedded migrations run automatically; do not also apply standalone SQL files.
5. Use the HTTPS proxy origin configured in SFD_PUBLIC_BASE_URL (example https://localhost:8443). Its local certificate needs appropriate browser trust. Mixing an HTTP backend URL with an HTTPS public origin will not work for secure session cookies/origin checks.

Compose storage credentials now match its MinIO variables. Cleanup configuration is forwarded to the backend. SFD_S3_* is the runtime naming; see [usage](USAGE.md).

## Before any public deployment

Review trusted TLS certificates, canonical origin/host routing, ingress/body/rate limits, private database/storage connectivity, restricted service credentials, secrets rotation, backup/restore, temporary hash disk capacity, resource limits and representative file sizes. No public deployment is authorised by this review.

The runtime uses Go 1.26.9 and Alpine 3.24 with the real OpenSSL-linked C utility and libcrypto3. It runs as an unprivileged user. CI builds and exercises this exact Dockerfile, separately from unit tests. See [tracker](TRACKER.md) for actual evidence and host blockers; historical production-ready statements do not certify this configuration.

## Community storage maintenance decision

The historical MinIO image is no longer publicly pullable. Development/tests build [Dockerfile.storage](../Dockerfile.storage) from exact official community source commits, with licenses retained. Both upstream community repositories are archived. This preserves S3/MinIO validation without changing vendor or requiring paid services, but production maintenance and version adoption require an Owner/Aether decision. No existing storage was upgraded.
