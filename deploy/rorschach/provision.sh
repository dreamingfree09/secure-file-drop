#!/bin/sh
set -eu
umask 077

endpoint=https://storage:9000
ca=/trust/storage-ca.pem
bucket=sfd-files

rc_checked() {
    rc --ca-cert "$ca" "$@" >/dev/null 2>&1
}

fail() {
    printf '%s\n' "$1" >&2
    exit 1
}

ready=false
for _ in $(seq 1 30); do
    if curl --fail --silent --cacert "$ca" --max-time 2 "$endpoint/health/ready" -o /dev/null; then
        ready=true
        break
    fi
    sleep 2
done
[ "$ready" = true ] || fail 'Storage TLS readiness did not pass'
rc_checked alias set root "$endpoint" "$R1_S3_ROOT_KEY" "$R1_S3_ROOT_SECRET" || fail 'Root authentication failed'
case "${1:-}" in
    initialize)
        rc_checked mb "root/$bucket" || fail 'Fresh bucket creation failed'
        sed 's/${BUCKET}/sfd-files/g' /opt/sfd/app-policy-template.json >/tmp/app-policy.json
        rc_checked admin user add root/ "$R1_S3_APP_KEY" "$R1_S3_APP_SECRET" || fail 'Scoped user creation failed'
        rc_checked admin policy create root/ sfd-runtime /tmp/app-policy.json || fail 'Scoped policy creation failed'
        rc_checked admin policy attach root/ sfd-runtime --user "$R1_S3_APP_KEY" || fail 'Scoped policy attachment failed'
        ;;
    verify) ;;
    *) fail 'Expected initialize or verify' ;;
esac
rc_checked alias set app "$endpoint" "$R1_S3_APP_KEY" "$R1_S3_APP_SECRET" || fail 'Application authentication failed'

object="rorschach-probe-$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
printf 'Secure File Drop Rorschach storage attestation\n' >/tmp/payload
rc_checked cp /tmp/payload "app/$bucket/$object" || fail 'Signed canary write failed'
# After a positively acknowledged create, remove only this invocation's random key.
trap 'rc_checked rm "app/$bucket/$object" || printf "%s\n" "Signed canary cleanup failed" >&2' EXIT
trap 'exit 1' HUP INT TERM
rc --ca-cert "$ca" cat "app/$bucket/$object" >/tmp/readback 2>/dev/null && cmp -s /tmp/payload /tmp/readback ||
    fail 'Signed canary bytes did not match'

expect_denied() {
    status=$(curl --silent --cacert "$ca" --max-time 10 -o /dev/null -w '%{http_code}' "$@") ||
        fail 'Anonymous denial probe transport failed'
    [ "$status" = 403 ] || fail 'Anonymous denial probe did not return HTTP 403'
}

expect_denied "$endpoint/$bucket/$object"
expect_denied --head "$endpoint/$bucket/$object"
expect_denied "$endpoint/$bucket?list-type=2"
expect_denied -X PUT --data-binary @/tmp/payload "$endpoint/$bucket/$object"
expect_denied -X DELETE "$endpoint/$bucket/$object"
rc --ca-cert "$ca" cat "app/$bucket/$object" >/tmp/readback 2>/dev/null && cmp -s /tmp/payload /tmp/readback ||
    fail 'Canary preservation check failed after denied requests'
trap - EXIT HUP INT TERM
rc_checked rm "app/$bucket/$object" || fail 'Signed canary cleanup failed'
printf '%s\n' 'PASS: signed exact-byte readback, five anonymous denials and owned canary removal'
