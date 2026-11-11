"""Strict real HTTPS + PostgreSQL managed link revocation on disposable RustFS."""
from hashlib import sha256
import json
import os
from pathlib import Path
import sys
from urllib.parse import parse_qs, urlencode, urlsplit
from uuid import UUID

import https_acceptance as https

ROOT = Path(os.environ["R1_RUNTIME"])
FILE = ROOT / "synthetic-file-id"
LEGACY = ROOT / "managed-fixture-legacy-link"
REVOKED = ROOT / "managed-fixture-revoked-link"


def login(username):
    https.COOKIE = ""
    _, headers = https.json_request("POST", "/login", 200, {
        "username": username, "password": os.environ["SFD_R1_FIXTURE_USER_PASSWORD"]
    })
    https.COOKIE = headers["Set-Cookie"].split(";", 1)[0]
    identity, _ = https.request("GET", "/me", 200)
    assert json.loads(identity)["is_admin"] is False


def raw_request(method, target, code, body=None):
    path = urlsplit(target)
    assert path.scheme == "https" and path.hostname == https.HOST and path.port == https.PORT
    return https.request(method, path.path + ("?" + path.query if path.query else ""), code, body)


def legacy_before():
    login("fixture")
    file_id = FILE.read_text(encoding="ascii")
    body, _ = https.json_request("POST", "/links", 200, {"id": file_id, "ttl_seconds": 3600})
    link = json.loads(body)
    assert urlsplit(link["url"]).path == "/download" and not link.get("link_id")
    LEGACY.write_text(link["url"], encoding="ascii")


def after_upgrade():
    login("fixture")
    status, _ = https.request("GET", "/config", 200)
    assert json.loads(status)["managed_links_enabled"] is True
    legacy = LEGACY.read_text(encoding="ascii")
    payload, _ = raw_request("GET", legacy, 200)
    assert payload == https.PAYLOAD and sha256(payload).hexdigest() == sha256(https.PAYLOAD).hexdigest()
    file_id = FILE.read_text(encoding="ascii")
    body, _ = https.json_request("POST", "/links", 200, {
        "id": file_id, "ttl_seconds": 3600, "share_page": True,
    })
    issued = json.loads(body)
    link_id = issued["link_id"]
    assert str(UUID(link_id)) == link_id
    signed = issued["url"]
    parts = urlsplit(signed)
    assert parts.path == "/share" and len(parse_qs(parts.query)["token"]) == 1
    token = parse_qs(parts.query)["token"][0]
    listing, _ = https.request("GET", "/managed-links?file_id=" + file_id, 200)
    rows = json.loads(listing)
    assert any(row["id"] == link_id and row["file_id"] == file_id and
               row.get("revoked_at") is None for row in rows)
    assert token.encode("ascii") not in listing and b"token_sha256" not in listing
    https.COOKIE = ""
    generic, _ = raw_request("GET", signed, 200)
    assert b"/share-download" in generic and https.PAYLOAD not in generic and b"rorschach-fixture.txt" not in generic
    form = urlencode({"token": token}).encode()
    downloaded, headers = https.request("POST", "/share-download", 200, form, "application/x-www-form-urlencoded")
    assert downloaded == https.PAYLOAD and headers.get("Content-Disposition", "").startswith("attachment")
    login("managed-other")
    listing, _ = https.request("GET", "/managed-links", 200)
    assert json.loads(listing) == []
    https.request("DELETE", "/managed-links/" + link_id, 404)
    https.json_request("POST", "/links", 404, {"id": file_id, "ttl_seconds": 300})
    login("fixture")
    https.request("DELETE", "/managed-links/" + link_id, 204)
    https.request("DELETE", "/managed-links/" + link_id, 204)
    listing, _ = https.request("GET", "/managed-links?file_id=" + file_id, 200)
    assert any(row["id"] == link_id and row.get("revoked_at") for row in json.loads(listing))
    https.COOKIE = ""
    raw_request("GET", "https://" + https.HOST + ":" + str(https.PORT) + "/download?token=" + token, 410)
    https.request("POST", "/share-download", 410, form, "application/x-www-form-urlencoded")
    REVOKED.write_text(token, encoding="ascii")
    login("fixture")
    second, _ = https.json_request("POST", "/links", 200, {
        "id": file_id, "ttl_seconds": 300, "share_page": True,
    })
    second_link = json.loads(second)["url"]
    second_token = parse_qs(urlsplit(second_link).query)["token"][0]
    assert second_token != token
    https.COOKIE = ""
    retry, _ = https.request("POST", "/share-download", 200,
                            urlencode({"token": second_token}).encode(), "application/x-www-form-urlencoded")
    assert retry == https.PAYLOAD
    print("PASS: schema15 persisted tokens, owner-only individual revocation, legacy compatibility, signed download bytes")


def after_restart():
    https.COOKIE = ""
    token = REVOKED.read_text(encoding="ascii")
    https.request("GET", "/download?token=" + token, 410)
    https.request("POST", "/share-download", 410,
                  urlencode({"token": token}).encode(), "application/x-www-form-urlencoded")
    login("fixture")
    listing, _ = https.request("GET", "/managed-links", 200)
    assert any(row.get("revoked_at") for row in json.loads(listing))
    print("PASS: revoked links fail closed after backend restart; durable state and byte integrity preserved")


if __name__ == "__main__":
    {"legacy-before": legacy_before, "after-upgrade": after_upgrade,
     "after-restart": after_restart}[sys.argv[1]]()
