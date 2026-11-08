"""Strict HTTPS checks for the owned Rorschach Compose resumable fixture."""

from concurrent.futures import ThreadPoolExecutor
from hashlib import sha256, shake_256
import json
import os
from pathlib import Path
import sys
import time
from urllib.parse import urlsplit
from uuid import uuid4

import https_acceptance as https

CHUNK = 32 << 20
API = "/uploads/resumable"
ROOT = Path(os.environ["R1_RUNTIME"])
TOTAL = CHUNK + 73


def login(name):
    https.COOKIE = ""
    _, headers = https.json_request("POST", "/login", 200, {
        "username": name, "password": os.environ["SFD_R1_FIXTURE_USER_PASSWORD"],
    })
    https.COOKIE = headers["Set-Cookie"].split(";", 1)[0]


def request(method, path, status, body=None, headers=None):
    return json.loads(https.request(method, path, status, body, headers=headers)[0])


def create(name, size, request_id=None, status=201):
    body = {"request_id": str(request_id or uuid4()), "name": name,
            "content_type": "application/octet-stream", "size_bytes": size}
    raw, _ = https.json_request("POST", API, status, body)
    return json.loads(raw) if status == 201 else None


def poll(id, state, seconds=150):
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        result = request("GET", API + "/" + id, 200)
        if result["state"] == state:
            return result
        if result["state"] == "quarantined":
            raise AssertionError("unexpected integrity quarantine")
        time.sleep(.5)
    raise AssertionError("resumable worker did not reach " + state)


def chunk_bytes(index, length):
    return shake_256(f"sfd-resumable-fixture-{index}".encode()).digest(length)


def send(id, offset, part, status=200, digest=None):
    headers = {"Upload-Offset": str(offset), "Upload-SHA256": digest or sha256(part).hexdigest()}
    raw, _ = https.request("PATCH", API + "/" + id, status, part, "application/octet-stream", headers)
    return json.loads(raw) if status == 200 else None


def complete(id, status=202):
    raw, _ = https.request("POST", API + "/" + id + "/complete", status)
    return json.loads(raw) if status == 202 else None


def oversized(id):
    conn = https.connection()
    try:
        conn.putrequest("PATCH", API + "/" + id)
        conn.putheader("Cookie", https.COOKIE)
        conn.putheader("Origin", f"https://{https.HOST}:{https.PORT}")
        conn.putheader("Connection", "close")
        conn.putheader("Content-Type", "application/octet-stream")
        conn.putheader("Content-Length", str(CHUNK + 1))
        conn.putheader("Upload-Offset", "0")
        conn.putheader("Upload-SHA256", "0" * 64)
        conn.endheaders()
        assert conn.getresponse().status == 413
    finally:
        conn.close()


def before():
    https.request("GET", API, 401)
    login("r2limited")
    create("over-quota.bin", 1025, status=403)
    login("fixture")
    token = uuid4()
    u = create("r2-restart.bin", TOTAL, token)
    assert create("r2-restart.bin", TOTAL, token)["id"] == u["id"]
    create("conflicting-name.bin", TOTAL, token, 409)
    id = u["id"]
    poll(id, "open")
    complete(id, 409)
    send(id, 1, b"bad", 409)
    oversized(id)
    part = chunk_bytes(0, CHUNK)
    send(id, 0, part, 422, "0" * 64)
    assert request("GET", API + "/" + id, 200)["offset"] == 0
    with ThreadPoolExecutor(max_workers=2) as threads:
        offsets = list(threads.map(lambda _: send(id, 0, part)["offset"], range(2)))
    assert offsets == [CHUNK, CHUNK]
    other_part = bytearray(part)
    other_part[0] ^= 1
    send(id, 0, bytes(other_part), 409)
    login("r2other")
    https.request("GET", API + "/" + id, 404)
    https.request("DELETE", API + "/" + id, 404)
    login("fixture")
    pending = create("r2-expiry.bin", 1)
    poll(pending["id"], "open")
    https.request("POST", "/logout", 200)
    (ROOT / "r2-resume-id").write_text(id, encoding="ascii")
    (ROOT / "r2-expiry-id").write_text(pending["id"], encoding="ascii")
    print("PASS: authentication, quota, create replay, offset/length/checksum, concurrent replay and ownership")


def submit_final():
    login("fixture")
    id = (ROOT / "r2-resume-id").read_text(encoding="ascii")
    u = request("GET", API + "/" + id + "?parts=1", 200)
    assert u["offset"] == CHUNK and u["part_sha256"] == [sha256(chunk_bytes(0, CHUNK)).hexdigest()]
    assert send(id, CHUNK, chunk_bytes(1, 73))["offset"] == TOTAL
    assert complete(id)["state"] == "finalizing"
    print("PASS: committed progress survived backend restart and reauthentication")


def verify_download(id, size, expected):
    upload = poll(id, "complete", seconds=240)
    assert upload["offset"] == size and upload["sha256"] == expected
    assert complete(id)["state"] == "complete"
    raw, _ = https.json_request("POST", "/links", 200, {"id": id, "ttl_seconds": 300})
    target = urlsplit(json.loads(raw)["url"])
    assert (target.scheme, target.hostname, target.port, target.path) == ("https", https.HOST, https.PORT, "/download")
    conn = https.connection(timeout=120)
    try:
        conn.request("GET", target.path + "?" + target.query)
        response = conn.getresponse()
        assert response.status == 200 and response.headers.get("Content-Encoding") is None
        actual, count = sha256(), 0
        while block := response.read(1 << 20):
            count += len(block)
            actual.update(block)
        assert count == size and actual.hexdigest() == expected
    finally:
        conn.close()


def reconcile():
    login("fixture")
    id = (ROOT / "r2-resume-id").read_text(encoding="ascii")
    expected = sha256()
    expected.update(chunk_bytes(0, CHUNK))
    expected.update(chunk_bytes(1, 73))
    verify_download(id, TOTAL, expected.hexdigest())
    expired = (ROOT / "r2-expiry-id").read_text(encoding="ascii")
    poll(expired, "cancelled")
    pending = create("r2-cancel.bin", TOTAL)
    poll(pending["id"], "open")
    send(pending["id"], 0, chunk_bytes(0, CHUNK))
    request("DELETE", API + "/" + pending["id"], 202)
    poll(pending["id"], "cancelled")
    assert request("DELETE", API + "/" + pending["id"], 202)["state"] == "cancelled"
    print("PASS: uncertain finalization reconciled to exact native hash/bytes, expiry and idempotent multipart cancellation")


def large():
    login("fixture")
    size = 1 << 30
    id = create("r2-one-gib.bin", size)["id"]
    poll(id, "open")
    expected = sha256()
    for index in range(size // CHUNK):
        part = chunk_bytes(index, CHUNK)
        expected.update(part)
        assert send(id, index * CHUNK, part)["offset"] == (index + 1) * CHUNK
    complete(id)
    verify_download(id, size, expected.hexdigest())
    print("PASS: 1 GiB of generated bytes uploaded in 32 verified requests and downloaded with matching SHA-256")


if __name__ == "__main__":
    {"before": before, "submit-final": submit_final, "reconcile": reconcile, "large": large}[sys.argv[1]]()
