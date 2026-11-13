"""No-email registration acceptance; only uniquely owned GitHub-hosted resources."""
from concurrent.futures import ThreadPoolExecutor
from hashlib import sha256
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
from urllib.parse import parse_qs, urlsplit

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'rorschach'))
import https_acceptance as https

ROOT = Path(os.environ['R1_RUNTIME'])
PROJECT = os.environ['SFD_R4_PROJECT']
assert re.fullmatch(r'sfd-rorschach-[a-f0-9]{12}', PROJECT)
assert os.environ.get('GITHUB_ACTIONS') == 'true' and os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted'
COMPOSE = ['docker', 'compose', '--env-file', '/dev/null', '-f', 'deploy/rorschach/compose.yml', '-p', PROJECT]
COOKIE = ''
PASSWORD = os.environ['SFD_R1_FIXTURE_USER_PASSWORD']


def command(args, *, data=None):
    result = subprocess.run(args, input=data, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, check=False)
    if result.returncode:
        raise RuntimeError('owned fixture command failed; private diagnostics withheld')
    return result.stdout.strip()


def sql(query):
    return command(COMPOSE + ['exec', '-T', 'postgres', 'sh', '-c',
        'export PGPASSWORD="$SFD_PG_APP_SECRET"; exec psql -h 127.0.0.1 -U sfd -d sfd -qAt -v ON_ERROR_STOP=1'], data=query)


def call(method, path, expected, value=None, *, cookie=None, origin=True):
    for attempt in range(2):
        connection = https.connection()
        headers = {'Content-Type': 'application/json'}
        if origin:
            headers['Origin'] = f'https://{https.HOST}:{https.PORT}'
        selected = COOKIE if cookie is None else cookie
        if selected:
            headers['Cookie'] = selected
        try:
            connection.request(method, path, body=json.dumps(value).encode() if value is not None else None, headers=headers)
            response = connection.getresponse()
            body = response.read()
            status, receipt = response.status, response.headers
        finally:
            connection.close()
        if status == 429 and expected != 429 and attempt == 0 and receipt.get('Retry-After') == '60':
            print('Fixture honours observed HTTP429 / Retry-After60; production limits unchanged', flush=True)
            time.sleep(61)
            continue
        assert status == expected, f'{method} {path.split("?")[0]}: expected {expected}, got {status}'
        return body, receipt
    raise AssertionError('bounded fixture wait exhausted')


def login(username, *, admin=False, database_admin=False):
    global COOKIE
    COOKIE = ''
    body, headers = call('POST', '/login', 200, {'username': username,
        'password': os.environ['SFD_R1_FIXTURE_PASSWORD'] if admin else PASSWORD})
    COOKIE = headers['Set-Cookie'].split(';', 1)[0]
    assert '; Secure' in headers['Set-Cookie'] and '; HttpOnly' in headers['Set-Cookie']
    identity, _ = call('GET', '/me', 200)
    parsed = json.loads(identity)
    assert parsed['is_admin'] is (admin or database_admin) and parsed['environment_admin'] is admin
    return parsed


def invitation():
    data, _ = call('POST', '/admin/registration-invitations', 201, {'ttl_seconds': 3600})
    receipt = json.loads(data)
    url = urlsplit(receipt['url'])
    assert url.scheme == 'https' and url.hostname == https.HOST and url.port == https.PORT
    assert url.path == '/join' and not url.query and not url.username
    token = parse_qs(url.fragment)['token'][0]
    assert re.fullmatch('[a-f0-9]{64}', token)
    assert sql("SELECT token_sha256 FROM registration_invitations WHERE id='" + receipt['id'] + "'") == sha256(token.encode()).hexdigest()
    listing, _ = call('GET', '/admin/registration-invitations', 200)
    assert token.encode() not in listing and b'token_sha256' not in listing and b'url' not in listing
    return receipt['id'], token


def credentials(token, username):
    return {'token': token, 'username': username, 'email': username + '@example.invalid', 'password': PASSWORD}


def redeem(token, username, expected=201, **extra):
    body, headers = call('POST', '/registration-invitations/redeem', expected,
                          {**credentials(token, username), **extra}, cookie='')
    assert not headers.get('Set-Cookie') and token.encode() not in body and PASSWORD.encode() not in body
    if expected == 201:
        result = json.loads(body)
        assert result['registration_method'] == 'administrator_invitation' and result['email_verified'] is False
    return body


def identities():
    return command(COMPOSE + ['ps', '-q', 'postgres', 'storage']).splitlines()


def restart_backend():
    before = identities()
    assert len(before) == 2
    for container in before:
        assert command(['docker', 'inspect', '--format', '{{index .Config.Labels "com.docker.compose.project"}}', container]) == PROJECT
    command(COMPOSE + ['stop', 'proxy', 'backend'])
    command(COMPOSE + ['up', '-d', '--no-build', '--force-recreate', '--no-deps', '--wait', '--wait-timeout', '180', 'backend', 'proxy'])
    assert identities() == before
    call('GET', '/ready', 200, cookie='')


def main():
    assert sql('SELECT version FROM schema_migrations') == '16'
    assert sql('SELECT owner FROM r4_fixture_ownership') == PROJECT
    login('admin', admin=True)
    assert json.loads(call('GET', '/config', 200)[0])['registration_invitations_enabled'] is True
    call('POST', '/register', 403, {'email': 'unused@example.invalid', 'username': 'unused', 'password': PASSWORD})
    private_admin = COOKIE
    primary, token = invitation()
    # GET/HEAD contains no invitation state and consumes nothing.
    page, headers = call('GET', '/join', 200, cookie='')
    assert token.encode() not in page and b'<form' in page
    assert headers['Referrer-Policy'] == 'no-referrer' and 'no-store' in headers['Cache-Control']
    call('HEAD', '/join', 200, cookie='')
    assert sql("SELECT redeemed_at IS NULL FROM registration_invitations WHERE id='" + primary + "'") == 't'
    call('POST', '/registration-invitations/redeem', 403, credentials(token, 'r4_missing_origin'), cookie='', origin=False)
    redeem(token, 'r4_bad_role', 400, is_admin=True)
    redeem(token, 'r4_bad_quota', 400, storage_quota_bytes=1 << 50)
    redeem(token, 'admin', 400)
    redeem(token, 'r4_short_password', 400, password='7')
    redeem('0' * 64, 'r4_unknown', 410)
    redeem(token, 'r4_api_user')
    redeem(token, 'r4_replay', 410)
    login('r4_api_user')
    call('GET', '/admin/registration-invitations', 403)
    call('POST', '/admin/registration-invitations', 403, {'ttl_seconds': 3600})
    call('DELETE', '/admin/registration-invitations/' + primary, 403)
    call('GET', '/admin/files', 403)
    # The inherited empty Go slice is encoded as JSON null, not an array.
    # Preserve that API contract and independently require no owned rows.
    listing = json.loads(call('GET', '/user/files', 200)[0])
    assert listing is None or listing == []
    assert sql("SELECT count(*) FROM files f JOIN users u ON u.id=f.user_id WHERE u.username='r4_api_user'") == '0'
    assert json.loads(call('GET', '/quota', 200)[0])['storage_quota_bytes'] == 10 << 30
    call('POST', '/logout', 200)
    call('GET', '/me', 401)
    call('POST', '/login', 401, {'username': 'r4_api_user@example.invalid', 'password': PASSWORD}, cookie='')
    call('POST', '/login', 401, {'username': 'r4_legacy_unverified', 'password': PASSWORD}, cookie='')
    login('r4_database_admin', database_admin=True)
    # A verified database administrator remains an administrator, but cannot
    # impersonate the independent environment issuer.
    assert json.loads(call('GET', '/me', 200)[0])['environment_admin'] is False
    call('POST', '/admin/registration-invitations', 403, {'ttl_seconds': 3600})
    login('admin', admin=True)
    duplicate_id, duplicate = invitation()
    redeem(duplicate, 'r4_api_user', 409)
    assert sql("SELECT redeemed_at IS NULL FROM registration_invitations WHERE id='" + duplicate_id + "'") == 't'
    redeem(duplicate, 'r4_duplicate_recovered')
    concurrent_id, concurrent = invitation()
    def race(name):
        connection = https.connection()
        try:
            connection.request('POST', '/registration-invitations/redeem', json.dumps(credentials(concurrent, name)),
                {'Origin': f'https://{https.HOST}:{https.PORT}', 'Content-Type': 'application/json'})
            response = connection.getresponse()
            response.read()
            return response.status
        finally:
            connection.close()
    # Pace through the ordinary limiter first; it is never raised or bypassed.
    print('Beginning two concurrent ordinary redemption requests', flush=True)
    time.sleep(61)
    with ThreadPoolExecutor(max_workers=2) as pool:
        statuses = sorted(pool.map(race, ['r4_race_one', 'r4_race_two']))
    assert statuses == [201, 410], statuses
    assert sql("SELECT count(*) FROM users WHERE registration_invitation_id='" + concurrent_id + "'") == '1'
    print('PASS: concurrent redemption201/410, exactly one ordinary account; invalid/extra role/quota fields, replay, identity/session/owner isolation', flush=True)
    expired_id, expired = invitation()
    sql("UPDATE registration_invitations SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 hour' WHERE id='" + expired_id + "'")
    redeem(expired, 'r4_expired', 410)
    revoked_id, revoked = invitation()
    call('DELETE', '/admin/registration-invitations/' + revoked_id, 204)
    call('DELETE', '/admin/registration-invitations/' + revoked_id, 204)
    redeem(revoked, 'r4_revoked', 410)
    rollback_id, rollback_token = invitation()
    sql('REVOKE INSERT ON users FROM sfd')
    try:
        redeem(rollback_token, 'r4_permission_recovered', 503)
    finally:
        sql('GRANT INSERT ON users TO sfd')
    assert sql("SELECT redeemed_at IS NULL FROM registration_invitations WHERE id='" + rollback_id + "'") == 't'
    redeem(rollback_token, 'r4_permission_recovered')
    deferred_id, deferred_token = invitation()
    sql("""CREATE FUNCTION r4_fail_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    RAISE EXCEPTION 'owned registration fixture rejects deferred commit'; END $$;
    CREATE CONSTRAINT TRIGGER r4_fail_commit AFTER INSERT ON registration_invitation_audit
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.event='redeemed') EXECUTE FUNCTION r4_fail_commit();""")
    try:
        body = redeem(deferred_token, 'r4_deferred_recovered', 503)
        assert json.loads(body)['error'] == 'outcome_unconfirmed'
    finally:
        sql('DROP TRIGGER r4_fail_commit ON registration_invitation_audit; DROP FUNCTION r4_fail_commit();')
    assert sql("SELECT count(*) FROM users WHERE username='r4_deferred_recovered'") == '0'
    assert sql("SELECT redeemed_at IS NULL FROM registration_invitations WHERE id='" + deferred_id + "'") == 't'
    redeem(deferred_token, 'r4_deferred_recovered')
    print('PASS: expiry/revocation and real SQL INSERT/deferred-COMMIT failures preserve atomic account/invitation state', flush=True)
    # True transaction outcome uncertainty: the production handler uses a real
    # PostgreSQL transaction through a test-only reporting driver. No app hook.
    executable = ROOT / 'registration-transaction-tests'
    subprocess.run(['go', 'test', '-c', '-o', str(executable), './tests/registration'],
                   env={**os.environ, 'CGO_ENABLED': '0'}, check=True)
    executable.chmod(0o555)
    network = PROJECT + '_database'
    assert command(['docker', 'network', 'inspect', '--format', '{{index .Labels "com.docker.compose.project"}}', network]) == PROJECT
    assert command(['docker', 'network', 'inspect', '--format', '{{.Internal}}', network]) == 'true'
    test_name = PROJECT + '-registration-transactions'
    assert not command(['docker', 'ps', '-aq', '--filter', 'name=^/' + test_name + '$'])
    image = command(['docker', 'image', 'inspect', '--format', '{{.Id}}', PROJECT + '-backend'])
    assert re.fullmatch(r'sha256:[a-f0-9]{64}', image)
    subprocess.run(['docker', 'run', '--rm', '--name', test_name, '--label', 'sfd.registration.fixture=' + PROJECT,
        '--network', network, '--read-only', '--user', '10001:10001', '--cap-drop', 'ALL',
        '--security-opt', 'no-new-privileges', '--tmpfs', '/tmp:rw,noexec,nosuid,size=32m',
        '--env', 'SFD_R4_TEST_DSN', '--env', 'SFD_R4_TEST_OWNER', '--env', 'SFD_R1_FIXTURE_USER_PASSWORD',
        '--env', 'GITHUB_ACTIONS', '--env', 'RUNNER_ENVIRONMENT',
        '--mount', 'type=bind,source=' + str(executable) + ',target=/registration-tests,readonly',
        '--entrypoint', '/registration-tests', image, '-test.v', '-test.run', '^TestRegistrationUnknownCommit$'],
        check=True, timeout=120)
    login('r4_commit_before')
    login('r4_commit_after')
    print('PASS: both real PostgreSQL pre-COMMIT rollback and post-COMMIT lost acknowledgement reconciled; actual runtime login succeeds', flush=True)
    login('admin', admin=True)
    subprocess.run(['node', 'tests/registration/browser.cjs', 'journey'], check=True)
    # Keep capabilities in process memory across a genuine backend recreation.
    restart_id, restart_token = invitation()
    restart_backend()
    call('POST', '/login', 200, {'username': 'fixture', 'password': PASSWORD}, cookie='')
    redeem(token, 'r4_restart_replay', 410)
    redeem(revoked, 'r4_restart_revoked', 410)
    redeem(restart_token, 'r4_after_restart')
    login('r4_after_restart')
    assert sql("SELECT bool_and(email_verified=FALSE AND is_admin=FALSE AND storage_quota_bytes=10737418240 AND verification_token IS NULL) FROM users WHERE registration_method='administrator_invitation'") == 't'
    assert sql("SELECT count(*) FROM registration_invitation_audit WHERE event='redeemed'") == sql("SELECT count(*) FROM users WHERE registration_method='administrator_invitation'")
    assert sql("SELECT count(*) FROM users WHERE username='r4_browser_user' AND registration_method='administrator_invitation'") == '1'
    login('admin', admin=True)
    for _ in range(20):
        connection = https.connection()
        try:
            connection.request('POST', '/admin/registration-invitations', json.dumps({'ttl_seconds': 3600}),
                {'Origin': f'https://{https.HOST}:{https.PORT}', 'Content-Type': 'application/json', 'Cookie': COOKIE})
            response = connection.getresponse(); response.read()
            if response.status == 429:
                assert response.headers['Retry-After'] == '3600'
                break
            assert response.status == 201
        finally:
            connection.close()
    else:
        raise AssertionError('durable issuance limit was not enforced')
    print('PASS: restart/replay/revocation persistence, unverified ordinary provenance/quota/audit, durable20/hour issuer limit', flush=True)
    # No plaintext capability is written to disk or stdout by this harness.
    assert primary and restart_id and private_admin


if __name__ == '__main__':
    main()
