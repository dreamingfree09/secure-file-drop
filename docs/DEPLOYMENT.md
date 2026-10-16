# Deployment

The checked-in Compose file is a local development configuration. Its published ports bind loopback; it uses persistent development volumes. Do not use it as a test harness or reuse existing volumes for validation. [The full-stack test](../tests/e2e/e2e_test.go) provisions its own disposable resources.

## Fresh local development only

1. Copy .env.example to .env and replace placeholders with independently generated strong passwords/secrets. Set a free, unique Compose project name with -p for the fresh stack. Do not start against an existing database/storage unintentionally.
2. Set SFD_MAX_REQUEST_BYTES to SFD_MAX_UPLOAD_BYTES plus 1048576 (sample 11534336 / 10485760); run scripts/validate-env.sh against the synthetic/fresh configuration. Start only dependencies: docker compose -p YOUR_UNIQUE_PROJECT up -d --build postgres minio.
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

Storage remains affected and is permitted only for trusted isolated synthetic development pending the [storage decision](DEEP_REVIEW_20261015.md#storage-decision-required--sourceadvisory-review-2026-10-16). Its API and console have no host ports; do not add routes or untrusted members to the internal storage network. Traefik explicitly selects the project default network for the backend. Compose allows 25 seconds for the application's bounded shutdown. None of these changes upgrade an existing installation or constitute a storage patch.


## RustFS preparation — isolated synthetic rehearsal only

Work ID: SFD-RUSTFS-ADOPTION-20261016-CODEX. The Owner authorised the 12-path preparation package subject to [Aether's conditions](https://github.com/dreamingfree09/secure-file-drop/pull/23#issuecomment-6223093103). RustFS remains a qualified candidate awaiting independent review and final vendor adoption approval. The default MinIO deployment is unchanged.

The opt-in [preparation configuration](../compose.rustfs-preparation.yml) pins official Linux amd64 RustFS Community 1.0.2 at rustfs/rustfs@sha256:c917317a76af3df35b596612a32e297b6a31a5c815e023fdf62ca78c2cf84201, source 2b243d8659497778c2406cc844a490565ec13287. It uses UID/GID 10001, dropped capabilities, no-new-privileges, a dedicated private bridge, fresh labelled volumes and only required loopback application/S3 test bindings. Signed administration shares the S3 port; there is no separately published console. The [pinned CLI definitions](https://github.com/rustfs/rustfs/blob/2b243d8659497778c2406cc844a490565ec13287/rustfs/src/config/cli.rs) and [protocol constants](https://github.com/rustfs/rustfs/blob/2b243d8659497778c2406cc844a490565ec13287/crates/config/src/constants/protocols.rs) define the explicitly disabled console, FTP, FTPS, SFTP and WebDAV settings. The fixture inspects actual listeners, privileges, manifest identity and executed version/binary SHA-256.

Run preparation through the [guarded rehearsal](../tests/e2e/rustfs_migration_rehearsal_test.go) on a local Linux Docker engine with Go/native prerequisites and the pinned sandboxed Chromium installed as described in README:

~~~sh
SFD_RUSTFS_REHEARSAL=1 SFD_REHEARSAL_REQUIRED=1 SFD_BROWSER=1 \
  go test ./tests/e2e -run '^TestRustFSMigrationRehearsal$' -v -count=1 -timeout 29m
~~~

The [.env template](../.env.rustfs-preparation.example) documents inputs, not usable credentials. The fixture generates each root/application/reader/outsider/database/session/download/administrator secret independently, renders the preparation Compose file without reading an existing application .env, verifies its constraints and uses its RustFS runtime environment in the owned containers. It orchestrates provisioning and readiness explicitly; plain Compose startup does not provision IAM/buckets or prove migration acceptance. The existing MinIO environment validator remains specific to the default stack.

Only the [single-bucket application policy](../tests/storage/rustfs-app-policy.json) is attached to the generated application identity. Root credentials remain in signed setup operations and storage; the backend receives only its scoped identity, and Chromium receives no storage credentials. The nested helper retains loopback, synthetic run/bucket and generated-identity guards; it is not a production provisioning tool. The dedicated workflow tests/verifies/scans that nested module explicitly and rejects imported affected-package/symbol findings. Read module-only residual advisories in the tracker; an exit code alone is not vulnerability certification.

Rehearsal order and acceptance:

1. Fresh MinIO/PostgreSQL/unchanged backend/native C resources; real authenticated uploads of empty/small/two 5 MiB + 17-byte fixtures, with one explicitly completed two-part object. A manifest binds UUID, original generated key, owner/username, resolved lifecycle, exact size and native SHA-256; an independent Go SHA-256 and byte comparison verifies every object.
2. Stop/drain the only source application within 25 seconds with cleanup disabled, take a consistent PostgreSQL custom-format checkpoint, stop source PostgreSQL and retain the source baseline. All later baseline access is read-only S3.
3. Logical S3 copy through scoped credentials, preserving keys and verifying bytes/count/hash against every metadata row. Normal signed permission checks reject anonymous, cross-bucket, outsider, reader-write and application administrative policy changes.
4. Restore the baseline into a fresh MinIO/PostgreSQL rollback pair before destination object writes, verifying real application operation against that pair.
5. Stop RustFS cleanly within 60 seconds; cold-copy only its owned volume read-only into a fresh owned volume as UID/GID 10001; restore PostgreSQL into another fresh database; start the unchanged application against this restored pair. Verify ownership/admin/cross-owner/anonymous access, signed-download bytes, Chromium and a new native-hashed upload. The separate existing candidate suite supplies actual proxy, cancellation and bounded shutdown regression evidence.
6. Add and delete bounded destination data, show that endpoint reversal leaves missing additions and stale deletions, quiesce and checkpoint the changed state, then logically reconcile into a third fresh rollback database/MinIO pair. Verify the resulting application, additions/deletions, owners/keys/size/hash and unchanged baseline checkpoint/object bytes.

Any phase failure stops subsequent phases. Resource labels are checked before stop/copy/removal; unknown ownership refuses cleanup. Success removes only fixture-owned resources. Failure stops owned containers and retains their source volumes/checkpoint for runner-local diagnosis. Sanitized manifests, checkpoint digest and test results are retained as CI artifacts for 14 days; raw checkpoints, generated configuration/secrets and full volumes are not published and do not survive hosted runner disposal. Local evidence is confined to a fresh fixture-owned directory. No host-wide cleanup, existing storage mount, Docker socket or remote engine is used.

HTTP is allowed only on this trusted isolated development/rehearsal network. Application-facing HTTPS termination does not encrypt backend-to-storage traffic. Production backend-to-storage TLS, capacity/maximum-file support, topology/maturity, security-update ownership and credential rotation, real-data/schema/ownership reconciliation, combined cross-host disaster recovery, production migration/cutover, public deployment, protected-branch integration and final vendor adoption all remain outstanding. This package is not public-production readiness.
