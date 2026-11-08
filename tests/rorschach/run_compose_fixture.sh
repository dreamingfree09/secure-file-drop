#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ "${GITHUB_ACTIONS:-}" == true && "${RUNNER_OS:-}" == Linux && "${RUNNER_ENVIRONMENT:-}" == github-hosted ]]
[[ "$(docker info --format '{{.OSType}}')" == linux ]]
[[ "$(docker context show)" == default && -z "${DOCKER_HOST:-}" && -z "${DOCKER_CONTEXT:-}" ]]
root=$(mktemp -d "${RUNNER_TEMP:?}/sfd-rorschach.XXXXXXXX")
export R1_RUNTIME="$root"
if [[ "${SFD_R2_HOSTED_FIXTURE:-0}" == 1 ]]; then
    export R1_RESUMABLE_MAX_BYTES=1073741824 R1_RESUMABLE_RESERVED_BYTES=2147483648
fi
export R1_SOURCE_REVISION
R1_SOURCE_REVISION=$(git rev-parse HEAD)
export R1_HOST=sfd.fixture.test R1_ORIGIN_PORT=18443 R1_PUBLIC_PORT_SUFFIX=:18443
export R1_PG_ADMIN R1_PG_APP R1_SESSION_SECRET R1_DOWNLOAD_SECRET R1_S3_ROOT_KEY R1_S3_APP_KEY R1_S3_ROOT_SECRET R1_S3_APP_SECRET
R1_PG_ADMIN=$(openssl rand -hex 32)
R1_PG_APP=$(openssl rand -hex 32)
R1_SESSION_SECRET=$(openssl rand -hex 32)
R1_DOWNLOAD_SECRET=$(openssl rand -hex 32)
R1_S3_ROOT_KEY=$(openssl rand -hex 10 | tr '[:lower:]' '[:upper:]')
R1_S3_APP_KEY=$(openssl rand -hex 10 | tr '[:lower:]' '[:upper:]')
R1_S3_ROOT_SECRET=$(openssl rand -hex 32)
R1_S3_APP_SECRET=$(openssl rand -hex 32)
export SFD_R1_FIXTURE_PASSWORD SFD_R1_FIXTURE_USER_PASSWORD R1_ADMIN_HASH
SFD_R1_FIXTURE_PASSWORD=$(openssl rand -hex 16)
SFD_R1_FIXTURE_USER_PASSWORD=$(openssl rand -hex 16)
R1_ADMIN_HASH=$(printf '%s\n' "$SFD_R1_FIXTURE_PASSWORD" | htpasswd -niBC 12 admin | cut -d: -f2-)
user_hash=$(printf '%s\n' "$SFD_R1_FIXTURE_USER_PASSWORD" | htpasswd -niBC 12 fixture | cut -d: -f2-)
mkdir "$root/origin-tls" "$root/storage-tls"

cert_pair() {
    local kind=$1 hostname=$2 cert=$3 key=$4
    openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=R1 $kind fixture CA" \
        -keyout "$root/$kind-ca.key" -out "$root/$kind-ca.pem" \
        -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign' 2>/dev/null
    openssl req -new -newkey rsa:2048 -nodes -subj "/CN=$hostname" -keyout "$key" -out "$root/$kind.csr" 2>/dev/null
    printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth\n' "$hostname" >"$root/$kind.ext"
    openssl x509 -req -in "$root/$kind.csr" -CA "$root/$kind-ca.pem" -CAkey "$root/$kind-ca.key" \
        -CAcreateserial -days 1 -extfile "$root/$kind.ext" -out "$cert" 2>/dev/null
    chmod 0755 "$(dirname "$cert")"
    chmod 0644 "$cert" "$root/$kind-ca.pem"
    sudo chown 10001:10001 "$key"
}

cert_pair storage storage "$root/storage-tls/rustfs_cert.pem" "$root/storage-tls/rustfs_key.pem"
cert_pair origin "$R1_HOST" "$root/origin-tls/fullchain.pem" "$root/origin-tls/key.pem"
project=sfd-rorschach-$(openssl rand -hex 6)
label="label=com.docker.compose.project=$project"
[[ -z "$(docker ps -aq --filter "$label")" && -z "$(docker volume ls -q --filter "$label")" && -z "$(docker network ls -q --filter "$label")" ]]
compose=(docker compose --env-file /dev/null -f deploy/rorschach/compose.yml -p "$project")
trap '"${compose[@]}" ps -a' ERR
"${compose[@]}" config --quiet
"${compose[@]}" build backend provision
"${compose[@]}" up -d postgres storage
"${compose[@]}" run --rm --no-deps provision initialize
"${compose[@]}" up -d --no-build --wait --wait-timeout 180 backend proxy
# Seed a verified ordinary identity only in this hosted disposable database.
# Real signup continues to require its existing email verification.
printf "INSERT INTO users (email, username, password_hash, email_verified, is_active) VALUES ('fixture@example.invalid', 'fixture', '%s', true, true);\n" "$user_hash" |
    "${compose[@]}" exec -T postgres sh -c 'export PGPASSWORD="$POSTGRES_PASSWORD"; exec psql -h 127.0.0.1 -U postgres -d sfd -q -v ON_ERROR_STOP=1'
fixture_sql() {
    "${compose[@]}" exec -T postgres sh -c 'export PGPASSWORD="$SFD_PG_APP_SECRET"; exec psql -h 127.0.0.1 -U sfd -d sfd -qAt -v ON_ERROR_STOP=1'
}
python3 -B tests/rorschach/https_acceptance.py create
if [[ "${SFD_R2_HOSTED_FIXTURE:-0}" == 1 ]]; then
    [[ "$(fixture_sql <<< 'SELECT version FROM schema_migrations')" == 14 ]]
    printf "INSERT INTO users (email, username, password_hash, email_verified, is_active,storage_quota_bytes) VALUES ('r2other@example.invalid','r2other','%s',true,true,NULL),('r2limited@example.invalid','r2limited','%s',true,true,1024);\n" "$user_hash" "$user_hash" | fixture_sql
    python3 -B tests/rorschach/resumable_acceptance.py before
fi
"${compose[@]}" stop proxy backend storage postgres
"${compose[@]}" start postgres storage
"${compose[@]}" run --rm --no-deps provision verify
"${compose[@]}" up -d --no-build --no-recreate --wait --wait-timeout 180 backend proxy
python3 -B tests/rorschach/https_acceptance.py reopen
if [[ "${SFD_R2_HOSTED_FIXTURE:-0}" == 1 ]]; then
    fixture_sql <<'SQL'
UPDATE r2_uploads SET expires_at = now() - interval '1 second' WHERE orig_name = 'r2-expiry.bin';
CREATE SEQUENCE r2_fixture_attempt;
CREATE FUNCTION r2_fixture_fail_publish() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.orig_name = 'r2-restart.bin' THEN
        PERFORM nextval('r2_fixture_attempt');
        RAISE EXCEPTION 'owned resumable fixture interrupts publication';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER r2_fixture_fail_publish BEFORE INSERT ON files FOR EACH ROW EXECUTE FUNCTION r2_fixture_fail_publish();
SQL
    python3 -B tests/rorschach/resumable_acceptance.py submit-final
    for attempt in $(seq 1 90); do
        [[ "$(fixture_sql <<< 'SELECT is_called FROM r2_fixture_attempt')" != t ]] || break
        sleep 1
    done
    [[ "$(fixture_sql <<< 'SELECT is_called FROM r2_fixture_attempt')" == t ]]
    [[ "$(fixture_sql <<< "SELECT state FROM r2_uploads WHERE orig_name='r2-restart.bin'")" == finalizing ]]
    [[ "$(fixture_sql <<< "SELECT count(*) FROM files WHERE orig_name='r2-restart.bin'")" == 0 ]]
    "${compose[@]}" stop proxy backend
    fixture_sql <<'SQL'
DROP TRIGGER r2_fixture_fail_publish ON files;
DROP FUNCTION r2_fixture_fail_publish();
DROP SEQUENCE r2_fixture_attempt;
SQL
    "${compose[@]}" up -d --no-build --force-recreate --no-deps --wait --wait-timeout 180 backend proxy
    python3 -B tests/rorschach/resumable_acceptance.py reconcile

    mkdir -p "$root/browser-home/.pki/nssdb"
    certutil -N --empty-password -d "sql:$root/browser-home/.pki/nssdb"
    certutil -A -n "SFD owned HTTPS fixture" -t 'C,,' -i "$root/origin-ca.pem" -d "sql:$root/browser-home/.pki/nssdb"
    dd if=/dev/urandom of="$root/r2-browser.bin" bs=1M count=32 status=none
    printf 'browser restart fixture\n' >>"$root/r2-browser.bin"
    node tests/rorschach/resumable_browser.cjs
fi
mkdir "$root/multipart"
dd if=/dev/urandom of="$root/multipart/multipart.bin" bs=1M count=65 status=none
chmod 0444 "$root/multipart/multipart.bin"
sudo chown 10001:10001 "$root/multipart"
"${compose[@]}" run --rm --no-deps --volume "$root/multipart:/fixture:rw" provision verify-multipart

# Consistent checkpoint: quiesce ingress, backend and object storage before capture.
"${compose[@]}" stop proxy backend storage
"${compose[@]}" exec -T postgres sh -c 'export PGPASSWORD="$SFD_PG_APP_SECRET"; exec pg_dump -h 127.0.0.1 -U sfd -d sfd -Fc' >"$root/postgres.dump"
"${compose[@]}" stop postgres
source_storage=$("${compose[@]}" ps -aq storage)
storage_image=$(docker inspect --format '{{.Image}}' "$source_storage")
source_volume="${project}_objects"
restore_project=sfd-rorschach-$(openssl rand -hex 6)
restore_volume="${restore_project}_objects"
restore=(docker compose --env-file /dev/null -f deploy/rorschach/compose.yml -p "$restore_project")
restore_label="label=com.docker.compose.project=$restore_project"
[[ "$restore_project" != "$project" && -z "$(docker ps -aq --filter "$restore_label")" &&
   -z "$(docker volume ls -q --filter "$restore_label")" && -z "$(docker network ls -q --filter "$restore_label")" ]]
[[ "$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' "$source_volume")" == "$project" &&
   "$(docker inspect --format '{{.State.Running}}' "$source_storage")" == false ]]
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
    --user 10001:10001 --entrypoint tar --mount "type=volume,source=$source_volume,target=/data,readonly" \
    "$storage_image" -C /data -cf - . >"$root/objects.tar"
sha256sum "$root/postgres.dump" "$root/objects.tar" >"$root/checkpoint.sha256"
sha256sum --check --status "$root/checkpoint.sha256"

# Restore into a separate project with new volumes using the identical local images.
for service in backend provision; do
    image_id=$(docker image inspect --format '{{.Id}}' "$project-$service")
    docker tag "$image_id" "$restore_project-$service"
done
"${restore[@]}" create --no-build storage
[[ "$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' "$restore_volume")" == "$restore_project" ]]
docker run --rm -i --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
    --user 10001:10001 --entrypoint tar --mount "type=volume,source=$restore_volume,target=/data" \
    "$storage_image" -C /data -xf - <"$root/objects.tar"
"${restore[@]}" up -d --wait --wait-timeout 60 postgres
"${restore[@]}" exec -T postgres sh -c 'export PGPASSWORD="$SFD_PG_APP_SECRET"; exec pg_restore --exit-on-error -h 127.0.0.1 -U sfd -d sfd' <"$root/postgres.dump"
"${restore[@]}" start storage
"${restore[@]}" run --rm --no-deps provision verify
"${restore[@]}" up -d --no-build --no-recreate --wait --wait-timeout 180 backend proxy
python3 -B tests/rorschach/https_acceptance.py restore
if [[ "${SFD_R2_HOSTED_FIXTURE:-0}" == 1 ]]; then
    docker_root=$(docker info --format '{{.DockerRootDir}}')
    available=$(sudo df -B1 --output=avail "$docker_root" | tail -1)
    [[ "$available" -ge 10737418240 ]]
    python3 -B tests/rorschach/resumable_acceptance.py large
fi
"${restore[@]}" stop
echo "PASS: paired database/object restore into a new project; original disposable services remain stopped"
