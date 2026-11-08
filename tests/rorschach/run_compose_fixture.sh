#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ "${GITHUB_ACTIONS:-}" == true && "${RUNNER_OS:-}" == Linux && "${RUNNER_ENVIRONMENT:-}" == github-hosted ]]
[[ "$(docker info --format '{{.OSType}}')" == linux ]]
[[ "$(docker context show)" == default && -z "${DOCKER_HOST:-}" && -z "${DOCKER_CONTEXT:-}" ]]
root=$(mktemp -d "${RUNNER_TEMP:?}/sfd-rorschach.XXXXXXXX")
export R1_RUNTIME="$root"
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
python3 -B tests/rorschach/https_acceptance.py create
"${compose[@]}" stop proxy backend storage postgres
"${compose[@]}" start postgres storage
"${compose[@]}" run --rm --no-deps provision verify
"${compose[@]}" up -d --no-build --no-recreate --wait --wait-timeout 180 backend proxy
python3 -B tests/rorschach/https_acceptance.py reopen
"${compose[@]}" stop
echo "PASS: disposable stack stopped; runner owns its containers, data and fixtures"
