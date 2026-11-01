"""Disposable fixture boundaries; no real SFD namespace or execution authority."""
import hashlib
import json
import os
import pathlib
import re
import stat
import tempfile
import uuid

SCHEMA = "SFD_DISPOSABLE_PKI_FIXTURE_V1"
LIMIT = 1048576

class Stop(ValueError):
    pass

def digest(data):
    return hashlib.sha256(data).hexdigest()

def create_workspace():
    nonce = str(uuid.uuid4())
    root = pathlib.Path(tempfile.gettempdir()) / ("sfd-pki-fixtures-" + nonce)
    root.mkdir(mode=0o700)
    write_new(root / "fixture.json", json.dumps({"schema": SCHEMA, "nonce": nonce,
              "synthetic_only": True}).encode())
    return root

def owned(root):
    root = pathlib.Path(root).absolute()
    if root.is_symlink() or getattr(root, "is_junction", lambda: False)():
        raise Stop("fixture-reparse")
    if root.parent.resolve() != pathlib.Path(tempfile.gettempdir()).resolve():
        raise Stop("fixture-parent")
    match = re.fullmatch(r"sfd-pki-(?:public-)?fixtures-([0-9a-f-]{36})", root.name)
    if not match or str(uuid.UUID(match[1])) != match[1]:
        raise Stop("fixture-name")
    meta = json.loads(read_small(root / "fixture.json"))
    if meta != {"schema": SCHEMA, "nonce": match[1], "synthetic_only": True}:
        raise Stop("fixture-marker")
    if os.name != "nt":
        s = root.stat()
        if s.st_uid != os.getuid() or stat.S_IMODE(s.st_mode) != 0o700:
            raise Stop("fixture-owner-mode")
    return root

def child(root, name):
    root = owned(root)
    if not re.fullmatch(r"[A-Za-z0-9_.-]{1,160}", name) or name in (".", ".."):
        raise Stop("fixture-child")
    p = root / name
    if p.is_symlink() or getattr(p, "is_junction", lambda: False)():
        raise Stop("fixture-child-reparse")
    return p

def read_small(path):
    path = pathlib.Path(path)
    if path.is_symlink() or getattr(path, "is_junction", lambda: False)():
        raise Stop("input-reparse")
    fd = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_BINARY", 0))
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode) or st.st_nlink != 1 or st.st_size > LIMIT:
            raise Stop("input-type-size-links")
        result = b""
        while len(result) <= LIMIT:
            b = os.read(fd, min(65536, LIMIT + 1 - len(result)))
            if not b:
                break
            result += b
        if len(result) > LIMIT:
            raise Stop("input-size")
        return result
    finally:
        os.close(fd)

def write_new(path, data):
    if len(data) > LIMIT:
        raise Stop("output-size")
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL |
                 getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_BINARY", 0), 0o600)
    try:
        with os.fdopen(fd, "wb", closefd=False) as stream:
            stream.write(data)
            stream.flush()
            os.fsync(fd)
    finally:
        os.close(fd)

def real_transition(*_args, **_kwargs):
    raise Stop("REAL_TRANSITION_DISABLED_PENDING_NEW_OWNER_AND_AETHER_AUTHORITY")

def assert_openssl_toolchain():
    pins=json.loads((pathlib.Path(__file__).parent/"toolchain.json").read_text())
    expected={"/usr/bin/openssl":pins["openssl_sha256"],**pins["openssl_library_SHA256"]}
    for path,sha in expected.items():
        if digest(pathlib.Path(path).read_bytes())!=sha:
            raise Stop("OpenSSL-executable-library-pin")
