"""Exact-byte integration against a disposable Rorschach Compose stack."""

from hashlib import sha256
import http.client
import json
import os
from pathlib import Path
import socket
import ssl
import sys
import time
from urllib.parse import urlencode, urlsplit

PAYLOAD = b"Rorschach native hashing fixture.\n" * 117
HOST = "sfd.fixture.test"
PORT = int(os.environ["R1_ORIGIN_PORT"])
CONTEXT = ssl.create_default_context(cafile=os.environ["R1_RUNTIME"] + "/origin-ca.pem")
COOKIE = ""


def connection(timeout=20):
    conn = http.client.HTTPSConnection(HOST, PORT, context=CONTEXT, timeout=timeout)
    conn._create_connection = lambda _address, *args: socket.create_connection(("127.0.0.1", PORT), *args)
    return conn


def request(method, path, status, body=None, content_type="application/json", headers=None):
    conn = connection()
    headers = {"Origin": f"https://{HOST}:{PORT}", **(headers or {})}
    if path.startswith("/download?"):
        headers["Accept-Encoding"] = "gzip"
    if body is not None:
        headers["Content-Type"] = content_type
    if COOKIE:
        headers["Cookie"] = COOKIE
    try:
        conn.request(method, path, body=body, headers=headers)
        result = conn.getresponse()
        data = result.read()
        if result.status != status:
            raise RuntimeError(f"{method} {path.split('?')[0]}: expected HTTP {status}, got {result.status}")
        return data, result.headers
    finally:
        conn.close()


def json_request(method, path, status, value):
    return request(method, path, status, json.dumps(value).encode())


def main():
    global COOKIE
    for attempt in range(15):
        try:
            data, _ = request("GET", "/ready", 200)
            break
        except (OSError, RuntimeError):
            if attempt == 14:
                raise
            time.sleep(2)
    components = json.loads(data)["components"]
    assert all(components[name]["status"] == "ok" for name in ("postgres", "minio", "native_hash"))
    request("GET", "/me", 401)
    def login(username, password_name, is_admin):
        global COOKIE
        COOKIE = ""
        _, headers = json_request("POST", "/login", 200, {
            "username": username, "password": os.environ[password_name],
        })
        receipt = headers["Set-Cookie"]
        assert "; Secure" in receipt and "; HttpOnly" in receipt
        COOKIE = receipt.split(";", 1)[0]
        identity, _ = request("GET", "/me", 200)
        assert json.loads(identity)["is_admin"] is is_admin
        return COOKIE

    administrator = login("admin", "SFD_R1_FIXTURE_PASSWORD", True)
    ordinary = login("fixture", "SFD_R1_FIXTURE_USER_PASSWORD", False)
    request("GET", "/admin/files", 403)
    id_file = Path(os.environ["R1_RUNTIME"]) / "synthetic-file-id"
    if sys.argv[1] == "create":
        data, _ = json_request("POST", "/files", 201, {
            "orig_name": "rorschach-fixture.txt", "content_type": "text/plain", "size_bytes": len(PAYLOAD),
        })
        file_id = json.loads(data)["id"]
        body = (
            b'--sfd-r1-boundary\r\nContent-Disposition: form-data; name="file"; filename="rorschach-fixture.txt"'
            b'\r\nContent-Type: text/plain\r\n\r\n' + PAYLOAD + b'\r\n--sfd-r1-boundary--\r\n'
        )
        request("POST", "/upload?" + urlencode({"id": file_id}), 200, body,
                "multipart/form-data; boundary=sfd-r1-boundary")
        id_file.write_text(file_id, encoding="ascii")
    else:
        file_id = id_file.read_text(encoding="ascii")
    COOKIE = administrator
    data, _ = request("GET", "/admin/files", 200)
    files = json.loads(data)
    assert any(row["id"] == file_id and row["sha256_hex"] == sha256(PAYLOAD).hexdigest() for row in files)
    COOKIE = ordinary
    data, _ = json_request("POST", "/links", 200, {"id": file_id, "ttl_seconds": 300})
    link = urlsplit(json.loads(data)["url"])
    assert (link.scheme, link.hostname, link.port, link.path) == ("https", HOST, PORT, "/download")
    assert not link.username and not link.fragment
    COOKIE = ""
    result, headers = request("GET", link.path + "?" + link.query, 200)
    assert result == PAYLOAD
    assert headers["Content-Disposition"].startswith("attachment;")
    assert headers.get("Content-Encoding") is None
    print(f"PASS: {sys.argv[1]} strict HTTPS, login, native hash and byte-identical signed download")


if __name__ == "__main__":
    main()
