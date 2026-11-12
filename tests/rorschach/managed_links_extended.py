"""Owner pagination, permissions and fail-closed checks on disposable schema15."""
import base64
import json
import os
from pathlib import Path
import sys
import time
from urllib.parse import parse_qs, urlsplit

import https_acceptance as https
import managed_links_acceptance as managed


def states():
    managed.login('fixture')
    file_id = managed.FILE.read_text(encoding='ascii')
    endpoint = '/managed-links?file_id=' + file_id
    first, headers = https.request('GET', endpoint, 200)
    rows = json.loads(first)
    assert len(rows) == 50 and all(row['file_id'] == file_id for row in rows)
    assert all(row['state'] in ('active', 'expired', 'revoked') for row in rows)
    cursor = headers['X-Next-Cursor']
    second, tail_headers = https.request('GET', endpoint + '&before=' + cursor, 200)
    tail = json.loads(second)
    assert tail and not tail_headers.get('X-Next-Cursor')
    assert not {row['id'] for row in rows} & {row['id'] for row in tail}
    assert sum(row['state'] == 'expired' for row in rows + tail) == 53
    assert b'token_sha256' not in first + second
    https.request('GET', endpoint + '&before=invalid', 400)
    issued, _ = https.json_request('POST', '/links', 200, {'id': file_id, 'ttl_seconds': 1})
    short = json.loads(issued)
    short_path = urlsplit(short['url'])
    time.sleep(2)
    https.request('GET', short_path.path + '?' + short_path.query, 410)
    https.request('GET', short_path.path + '?' + short_path.query, 410, headers={'Range': 'bytes=0-7'})
    expired, _ = https.request('GET', endpoint, 200)
    assert any(row['id'] == short['link_id'] and row['state'] == 'expired' for row in json.loads(expired))
    receipt = json.loads((Path(os.environ['R1_RUNTIME']) / 'managed-browser-receipt.json').read_text())
    https.request('DELETE', '/managed-links/' + receipt['revoked'], 403,
                  headers={'Origin': 'https://foreign.example.invalid'})
    managed.login('managed-other')
    foreign, _ = https.request('GET', endpoint + '&before=' + cursor, 200)
    assert json.loads(foreign) == []
    https.COOKIE = ''
    signed = receipt['other'].replace('/share?', '/download?')
    https.request('GET', urlsplit(signed).path + '?' + urlsplit(signed).query + 'x', 401)
    print('PASS: real PostgreSQL keyset pagination, 53 expired records, private listing, foreign cursor and cross-origin denial')
    print('PASS: issued one-second token expires for real full/Range requests and server-reported metadata')


def database_failure():
    managed.login('fixture')
    body, _ = https.request('GET', '/managed-links', 503)
    assert b'managed sharing unavailable' in body and b'permission' not in body
    receipt = json.loads((Path(os.environ['R1_RUNTIME']) / 'managed-browser-receipt.json').read_text())
    signed = urlsplit(receipt['other'])
    token = parse_qs(signed.query)['token'][0]
    claims = json.loads(base64.urlsafe_b64decode(token.split('.')[0] + '==='))
    https.request('DELETE', '/managed-links/' + claims['link_id'], 503)
    https.COOKIE = ''
    https.request('GET', '/download?token=' + token, 503)
    https.request('GET', '/download?token=' + token, 503, headers={'Range': 'bytes=0-7'})
    print('PASS: real database SELECT denial fails listing and full/Range downloads closed without diagnostics')


def insert_failure():
    managed.login('fixture')
    body, _ = https.json_request('POST', '/links', 503, {
        'id': managed.FILE.read_text(encoding='ascii'), 'ttl_seconds': 300,
    })
    assert b'token' not in body and b'permission' not in body and b'http' not in body
    print('PASS: real database INSERT denial discloses no unrecorded signed link')


if __name__ == '__main__':
    {'states': states, 'database-failure': database_failure, 'insert-failure': insert_failure}[sys.argv[1]]()
