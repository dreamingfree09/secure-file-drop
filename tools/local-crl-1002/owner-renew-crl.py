#!/usr/bin/env python3
"""SFD CRL 0x1002 signing-only helper. Default check never signs or publishes."""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys

class Stop(Exception):
    pass

def require(ok, code):
    if not ok:
        raise Stop(code)

def digest(data):
    return hashlib.sha256(data).hexdigest()

def regular(path, private=False):
    path = Path(path)
    require(path.is_absolute(), "PATH_NOT_ABSOLUTE")
    for parent in [path, *path.parents]:
        require(not parent.is_symlink(), "SYMLINK")
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode), "NOT_REGULAR")
    if private:
        require(info.st_uid == os.getuid() and info.st_mode & 0o077 == 0,
                "PRIVATE_FILE_PERMISSIONS")
    return info

def read_bound(path, expected, private=False):
    regular(path, private)
    data = Path(path).read_bytes()
    require(digest(data) == expected, "PIN_MISMATCH")
    return data

def run_public(args):
    result = subprocess.run(args, capture_output=True, timeout=30, check=False)
    require(result.returncode == 0, "PUBLIC_OPENSSL_CHECK_FAILED")
    return result.stdout, result.stderr

def key_metadata(path):
    # Deliberately never open, read, hash or copy private key bytes.
    info = regular(path, True)
    return dict(mode=stat.S_IMODE(info.st_mode), uid=info.st_uid,
                gid=info.st_gid, bytes=info.st_size, mtime_ns=info.st_mtime_ns,
                inode=info.st_ino, device=info.st_dev)

def crl_fields(text, expected_number, now):
    values = dict(line.split("=", 1) for line in text.strip().splitlines())
    require(values.get("crlNumber") == expected_number, "CRL_NUMBER")
    require(values.get("issuer") == "CN = Secure File Drop Local Development CA",
            "CRL_ISSUER")
    parse = lambda value: dt.datetime.strptime(value, "%b %d %H:%M:%S %Y GMT").replace(tzinfo=dt.timezone.utc)
    start, end = parse(values["lastUpdate"]), parse(values["nextUpdate"])
    require(abs((now - start).total_seconds()) <= 600, "CRL_START")
    require(end - start == dt.timedelta(days=7), "CRL_LIFETIME")
    require(end - now >= dt.timedelta(days=6), "CRL_REMAINING")
    return dict(last_update=start.isoformat(), next_update=end.isoformat())

def check_context(context_path, expected_sha):
    raw = read_bound(context_path, expected_sha, True)
    context = json.loads(raw)
    require(context["work_id"] == "SFD-CRL-1002-PREPARATION-20261027", "WORK_ID")
    require(context["target_crl"] == "0x1002" and
            context["counter_before"] == "1002" and
            context["counter_after"] == "1003", "EXPECTED_SEQUENCE")
    require(digest(Path(__file__).read_bytes()) == context["helper_sha256"],
            "HELPER_IDENTITY")
    require(Path(context_path).parent == Path(context["output_dir"]), "CONTEXT_LOCATION")
    outdir = Path(context["output_dir"])
    info = outdir.lstat()
    require(not outdir.is_symlink() and stat.S_ISDIR(info.st_mode) and
            info.st_uid == os.getuid() and info.st_mode & 0o077 == 0,
            "PRIVATE_DIRECTORY")
    require(key_metadata(context["key_path"]) == context["key_metadata"],
            "KEY_METADATA_CHANGED")
    require(context["key_path"] not in [item["path"] for item in context["pins"].values()],
            "PRIVATE_KEY_MUST_NOT_BE_HASHED")
    for item in context["pins"].values():
        read_bound(item["path"], item["sha256"])
    counter = Path(context["pins"]["counter"]["path"]).read_text().strip()
    require(counter == context["counter_before"], "COUNTER_CHANGED")
    rows = Path(context["pins"]["index"]["path"]).read_text().splitlines()
    require(len(rows) == 2 and all(row.startswith("V\t") for row in rows),
            "LEDGER_CHANGED")
    openssl = context["openssl"]
    read_bound(openssl["path"], openssl["sha256"])
    publication = context["pins"]["published"]["path"]
    root = context["pins"]["root"]["path"]
    output, error = run_public([openssl["path"], "crl", "-inform", "DER",
                               "-in", publication, "-verify", "-CAfile", root,
                               "-noout", "-crlnumber"])
    require(b"verify OK" in error and output.strip() == b"crlNumber=0x1001",
            "CURRENT_CRL_NOT_AUTHENTIC")
    return context

def verify_acceptance(path, expected_sha, context_sha, helper_sha, now):
    require(path is not None and expected_sha is not None, "SIGNING_REVIEW_REQUIRED")
    acceptance = json.loads(read_bound(path, expected_sha, True))
    require(acceptance.get("signing_authorized") is True, "SIGNING_NOT_AUTHORIZED")
    require(acceptance.get("reviewer") == "Aether" and
            isinstance(acceptance.get("comment_id"), int) and
            acceptance["comment_id"] > 6454489528 and
            re.fullmatch("[0-9a-f]{64}", acceptance.get("comment_body_sha256", "")),
            "EXACT_REVIEW_BINDING_REQUIRED")
    require(acceptance.get("context_sha256") == context_sha and
            acceptance.get("helper_sha256") == helper_sha, "REVIEW_IDENTITY")
    until = dt.datetime.fromisoformat(acceptance["valid_until"])
    require(until.tzinfo is not None and now < until <= now + dt.timedelta(hours=24),
            "REVIEW_WINDOW")

def sign(context_path, context_sha, acceptance_path, acceptance_sha):
    now = dt.datetime.now(dt.timezone.utc)
    context = check_context(context_path, context_sha)
    verify_acceptance(acceptance_path, acceptance_sha, context_sha,
                      context["helper_sha256"], now)
    require(sys.stdin.isatty() and sys.stdout.isatty(), "LOCAL_TTY_REQUIRED")
    issuer = Path(context["pins"]["config"]["path"]).parent
    lock_path = issuer / "signing.lock"
    # Existing issuer-wide lock, no symlink following, no secret content.
    lock_fd = os.open(lock_path, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    with os.fdopen(lock_fd, "a") as lock:
        lock_info = os.fstat(lock.fileno())
        require(stat.S_ISREG(lock_info.st_mode) and lock_info.st_uid == os.getuid() and
                lock_info.st_mode & 0o077 == 0, "LOCK_PERMISSIONS")
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        context = check_context(context_path, context_sha)
        outdir = Path(context["output_dir"])
        names = ["started.json", "new-crl.pem", "new-crl.der",
                 "verification.json", "counter-before.txt", "counter-old-before.txt"]
        require(not any((outdir / name).exists() for name in names), "ONE_USE_OUTPUT_EXISTS")
        before = Path(context["pins"]["counter"]["path"]).read_bytes()
        old_counter = issuer / "crlnumber.old"
        require(old_counter.exists(), "OLD_COUNTER_MISSING")
        regular(old_counter)
        # OpenSSL normally overwrites crlnumber.old; preserve its preimage first.
        old = old_counter.read_bytes()
        with (outdir / "counter-before.txt").open("xb") as f:
            f.write(before)
        with (outdir / "counter-old-before.txt").open("xb") as f:
            f.write(old)
        with (outdir / "started.json").open("x") as f:
            json.dump({"context_sha256": context_sha, "utc": now.isoformat()}, f)
        pem, der = outdir / "new-crl.pem", outdir / "new-crl.der"
        print("Enter the EXISTING CA passphrase only at OpenSSL's hidden local prompt.")
        # PEM goes to an exclusive new file, passphrase goes directly to OpenSSL.
        # No passin, environment secret, password capture or publication.
        with pem.open("xb") as pem_file, open("/dev/tty", "rb+", buffering=0) as tty:
            result = subprocess.run(
                [context["openssl"]["path"], "ca", "-gencrl", "-config",
                 context["pins"]["config"]["path"]],
                stdin=tty, stdout=pem_file, stderr=tty, check=False)
            pem_file.flush()
            os.fsync(pem_file.fileno())
        require(result.returncode == 0, "SIGNING_FAILED_PRESERVE_STATE")
        output, error = run_public([context["openssl"]["path"], "crl",
                                   "-in", str(pem), "-verify", "-CAfile",
                                   context["pins"]["root"]["path"], "-noout",
                                   "-issuer", "-crlnumber", "-lastupdate", "-nextupdate"])
        require(b"verify OK" in error, "NEW_SIGNATURE")
        validity = crl_fields(output.decode("ascii"), "0x1002",
                              dt.datetime.now(dt.timezone.utc))
        run_public([context["openssl"]["path"], "verify", "-CAfile",
                    context["pins"]["root"]["path"], "-CRLfile", str(pem),
                    "-crl_check", "-verify_hostname", "localhost",
                    context["pins"]["leaf"]["path"]])
        counter = Path(context["pins"]["counter"]["path"]).read_text().strip()
        require(counter == "1003", "POST_COUNTER")
        for name, item in context["pins"].items():
            if name not in ("counter", "counter_old"):
                read_bound(item["path"], item["sha256"])
        require(key_metadata(context["key_path"]) == context["key_metadata"],
                "POST_KEY_METADATA")
        require(old_counter.read_bytes() == before, "POST_OLD_COUNTER")
        der_bytes, _ = run_public([context["openssl"]["path"], "crl",
                                  "-in", str(pem), "-outform", "DER"])
        with der.open("xb") as f:
            f.write(der_bytes)
            f.flush()
            os.fsync(f.fileno())
        _, error = run_public([context["openssl"]["path"], "crl", "-inform",
                               "DER", "-in", str(der), "-verify", "-CAfile",
                               context["pins"]["root"]["path"], "-noout"])
        require(b"verify OK" in error, "DER_SIGNATURE")
        report = dict(success=True, crl="0x1002", counter="1003",
                      der_sha256=digest(der_bytes), publication_changed=False,
                      key_bytes_read_by_helper=False, **validity)
        with (outdir / "verification.json").open("x") as f:
            json.dump(report, f, indent=2)
        print("SIGNED CRL READY. Signature, number, validity, ledger and active certificate verified. Publication unchanged; return to Codex.")

def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", required=True, type=Path)
    parser.add_argument("--context-sha256", required=True)
    parser.add_argument("--sign", action="store_true")
    parser.add_argument("--acceptance", type=Path)
    parser.add_argument("--acceptance-sha256")
    args = parser.parse_args()
    try:
        if args.sign:
            sign(args.context, args.context_sha256, args.acceptance, args.acceptance_sha256)
        else:
            check_context(args.context, args.context_sha256)
            print("PREPARATION CHECK PASS. No signing or publication performed.")
    except Exception as error:
        code = str(error) if isinstance(error, Stop) else type(error).__name__
        print("STOP: " + code + ". Preserve all outputs; do not retry signing.")
        return 1
    return 0

if __name__ == "__main__":
    sys.exit(main())
