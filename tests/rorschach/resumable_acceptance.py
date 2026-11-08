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
            "content_type": "application/octet-stream", "size_bytes": size,
            "manifest_sha256": manifest(size)}
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


def manifest(size):
    checksums = sha256()
    for i, offset in enumerate(range(0, size, CHUNK)):
        checksums.update(sha256(chunk_bytes(i, min(CHUNK, size - offset))).hexdigest().encode("ascii"))
    return checksums.hexdigest()


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
    upload = poll(id, "complete", seconds=240 if size <= 1 << 30 else 900)
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
    poll(expired, "canceled")
    pending = create("r2-cancel.bin", TOTAL)
    poll(pending["id"], "open")
    send(pending["id"], 0, chunk_bytes(0, CHUNK))
    request("DELETE", API + "/" + pending["id"], 202)
    poll(pending["id"], "canceled")
    assert request("DELETE", API + "/" + pending["id"], 202)["state"] == "canceled"
    conflict = create("r2-wrong-manifest.bin", 1)["id"]
    poll(conflict, "open")
    send(conflict, 0, bytes([chunk_bytes(0, 1)[0] ^ 1]))
    complete(conflict, 409)
    request("DELETE", API + "/" + conflict, 202)
    poll(conflict, "canceled")
    print("PASS: uncertain finalization reconciled to exact native hash/bytes, expiry and idempotent multipart cancellation")


def reject_cancel():
    login("fixture")
    id = (ROOT / "r2-resume-id").read_text(encoding="ascii")
    https.request("DELETE", API + "/" + id, 409)
    assert request("GET", API + "/" + id, 200)["state"] == "finalizing"
    print("PASS: cancellation cannot abort a possibly completed object")


def large():
    login("fixture")
    gib = int(sys.argv[2])
    assert gib in (1, 10)
    size = gib << 30
    parts = size // CHUNK
    id = create(f"r2-{gib}-gib.bin", size)["id"]
    poll(id, "open")
    expected = sha256()
    for index in range(parts):
        part = chunk_bytes(index, CHUNK)
        expected.update(part)
        assert send(id, index * CHUNK, part)["offset"] == (index + 1) * CHUNK
        if (index + 1) % max(8, parts // 4) == 0:
            print(f"Confirmed {index + 1} of {parts} part commits", flush=True)
    complete(id)
    verify_download(id, size, expected.hexdigest())
    print(f"PASS: {gib} GiB of generated bytes uploaded in {parts} verified requests and downloaded with matching SHA-256")


if __name__ == "__main__":
    {"before": before, "submit-final": submit_final, "reconcile": reconcile,
     "reject-cancel": reject_cancel, "large": large}[sys.argv[1]]()
