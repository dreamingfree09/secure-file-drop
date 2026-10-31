#!/usr/bin/env python3
"""Owner-attended CA key-unlock check. Never signs, exports a private key, or changes issuer files."""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import stat
import select
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
    require(path.is_absolute(), "ABSOLUTE_PATH_REQUIRED")
    for parent in [path, *path.parents]:
        require(not parent.is_symlink(), "SYMLINK")
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode), "NOT_REGULAR")
    if private:
        require(info.st_uid == os.getuid() and info.st_mode & 0o077 == 0,
                "PRIVATE_PERMISSIONS")
    return info

def key_metadata(path):
    # Never open, read, hash, export or copy private key bytes in Python.
    info = regular(path, True)
    return dict(mode=stat.S_IMODE(info.st_mode), uid=info.st_uid,
                gid=info.st_gid, bytes=info.st_size, mtime_ns=info.st_mtime_ns,
                inode=info.st_ino, device=info.st_dev)

def check_state(context):
    require(key_metadata(context["key_path"]) == context["key_metadata"],
            "KEY_METADATA_CHANGED")
    require(context["key_path"] not in [p["path"] for p in context["pins"]],
            "PRIVATE_KEY_MUST_NOT_BE_HASHED")
    for item in context["pins"]:
        regular(item["path"])
        require(digest(Path(item["path"]).read_bytes()) == item["sha256"],
                "PRESERVATION_PIN_CHANGED")
    regular(context["openssl"])
    require(digest(Path(context["openssl"]).read_bytes()) == context["openssl_sha256"],
            "OPENSSL_CHANGED")

def command(context, public_output):
    # Fixed operation: public-key output only. No ca/req/x509 signing or private-key output.
    return [context["openssl"], "pkey", "-in", context["key_path"], "-check",
            "-pubout", "-outform", "DER", "-out", str(public_output)]

def write_new(path, value):
    with Path(path).open("x") as file:
        json.dump(value, file, indent=2)
        file.flush()
        os.fsync(file.fileno())

def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", required=True, type=Path)
    parser.add_argument("--context-sha256", required=True)
    args = parser.parse_args()
    context = None
    child = None
    result = dict(success=False, signing_performed=False, issuer_changed=False,
                  password_captured=False, attempts=0)
    output_dir = None
    try:
        regular(args.context, True)
        raw = args.context.read_bytes()
        require(digest(raw) == args.context_sha256, "CONTEXT_HASH")
        context = json.loads(raw)
        require(context["kind"] == "SFD_CA_UNLOCK_CHECK_V1" and
                context["signing_authorized"] is False and
                context["max_attempts"] == 3, "NON_SIGNING_SCOPE")
        require(digest(Path(__file__).read_bytes()) == context["helper_sha256"],
                "HELPER_HASH")
        output_dir = Path(context["output_dir"])
        info = output_dir.lstat()
        require(args.context.parent == output_dir and not output_dir.is_symlink()
                and stat.S_ISDIR(info.st_mode) and info.st_uid == os.getuid()
                and info.st_mode & 0o077 == 0, "PRIVATE_OUTPUT_DIRECTORY")
        until = dt.datetime.fromisoformat(context["valid_until"])
        require(until.tzinfo is not None and
                dt.datetime.now(dt.timezone.utc) < until, "CHECK_WINDOW")
        require(sys.stdin.isatty() and sys.stdout.isatty(), "OWNER_LOCAL_TTY_REQUIRED")
        with open("/dev/tty", "rb+", buffering=0) as tty:
            require(os.isatty(tty.fileno()), "CONTROLLING_TTY_REQUIRED")
        check_state(context)
        require(not any((output_dir / name).exists() for name in
                        ["started.json", "result.json"]), "ONE_USE_CHECK_SPENT")
        write_new(output_dir / "started.json",
                  dict(utc=dt.datetime.now(dt.timezone.utc).isoformat(),
                       context_sha256=args.context_sha256,
                       signing_authorized=False))
        print("\033]0;SFD CA Key Check - NO SIGNING\007", end="", flush=True)
        print("NON-SIGNING CA KEY CHECK. Enter candidates only at OpenSSL's hidden prompt.",
              flush=True)
        print("No password is recorded. Existing key, ledger, counter, CRL and trust remain unchanged.",
              flush=True)
        for number in range(1, 4):
            require(dt.datetime.now(dt.timezone.utc) < until, "CHECK_WINDOW")
            check_state(context)
            public = output_dir / ("check-%02d-public-output.bin" % number)
            with public.open("xb"):
                pass
            print("Local key check %d of 3. This does not sign anything." % number,
                  flush=True)
            result["attempts"] = number
            write_new(output_dir / ("attempt-%02d-started.json" % number),
                      dict(utc=dt.datetime.now(dt.timezone.utc).isoformat(),
                           operation="pkey-check-public-output-only"))
            with open("/dev/tty", "rb+", buffering=0) as tty:
                child = subprocess.Popen(command(context, public),
                                         stdin=tty, stdout=tty, stderr=tty,
                                         env={"PATH": "/usr/bin:/bin", "LANG": "C"})
                limit = min(180, (until - dt.datetime.now(dt.timezone.utc)).total_seconds())
                try:
                    status = child.wait(timeout=max(1, limit))
                except subprocess.TimeoutExpired:
                    child.terminate()
                    try:
                        child.wait(timeout=3)
                    except subprocess.TimeoutExpired:
                        child.kill()
                        child.wait(timeout=3)
                    raise Stop("LOCAL_INPUT_TIMEOUT")
            child = None
            check_state(context)
            regular(public, True)
            require(public.stat().st_size <= 16384, "PUBLIC_OUTPUT_BOUND")
            public_bytes = public.read_bytes()
            prefix = b"Key is valid\n"
            checked = public_bytes.startswith(prefix)
            public_hash = digest(public_bytes[len(prefix):]) if checked else None
            passed = status == 0 and checked and public_hash == context["expected_public_spki_sha256"]
            write_new(output_dir / ("attempt-%02d-result.json" % number),
                      dict(exit=status, matches_existing_CA=passed,
                           public_bytes=public.stat().st_size,
                           public_spki_sha256=public_hash if status == 0 else None,
                           issuer_unchanged=True))
            if passed:
                result.update(success=True, public_spki_sha256=public_hash)
                break
            require(status != 0, "VALID_KEY_WRONG_PUBLIC_IDENTITY")
            print("Key check did not unlock the expected CA. No signing or issuer change.",
                  flush=True)
            if number < 3:
                # Only a non-secret local control word, never a password, is read by Python.
                print("Type NEXT here to try another candidate, or STOP to finish: ", end="", flush=True)
                remaining = (until - dt.datetime.now(dt.timezone.utc)).total_seconds()
                require(remaining > 0 and select.select([sys.stdin], [], [], remaining)[0], "CHECK_WINDOW")
                action = sys.stdin.readline()
                if action.strip() != "NEXT":
                    result["stop_code"] = "OWNER_STOPPED_CHECK"
                    break
        check_state(context)
        result["utc"] = dt.datetime.now(dt.timezone.utc).isoformat()
        write_new(output_dir / "result.json", result)
        print("CA KEY CHECK PASS. Existing key matches the CA. No signing performed."
              if result["success"] else
              "CA KEY CHECK ENDED WITHOUT A MATCH. No signing performed.", flush=True)
        print("Return to Codex. Do not reopen or rerun this one-use check.", flush=True)
        return 0 if result["success"] else 1
    except BaseException as error:
        if child is not None and child.poll() is None:
            child.terminate()
            try:
                child.wait(timeout=3)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait(timeout=3)
        result["stop_code"] = str(error) if isinstance(error, Stop) else type(error).__name__
        result["utc"] = dt.datetime.now(dt.timezone.utc).isoformat()
        if context is not None:
            try:
                check_state(context)
                result["issuer_changed"] = False
            except Exception:
                result["issuer_changed"] = "UNVERIFIED"
        if output_dir is not None and not (output_dir / "result.json").exists():
            try:
                write_new(output_dir / "result.json", result)
            except Exception:
                pass
        print("STOP: " + result["stop_code"] + ". Preserve files; no signing performed.",
              flush=True)
        return 1

if __name__ == "__main__":
    sys.exit(main())
