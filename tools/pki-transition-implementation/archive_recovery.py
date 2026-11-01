"""Complete authenticated recovery of DISPOSABLE issuer fixtures. No real CA/archive entry point."""
import base64
import datetime
import hashlib
import hmac
import json
import os
import pathlib
import re
import secrets
import stat
from fence import Stop, child, create_workspace, digest, owned, read_small, write_new
from der import certificate, crl, seq
from crypto import pem
from publisher_fixture import publication_lock
from context import verify
from recovery_history import PREFIX, event, split_name, snapshot_history, validate as validate_history, write_history, inventory_digest, check_current

SCHEMA="SFD_SYNTHETIC_ARCHIVE_V3"
CORE={"ca.enc.pem","root.pem","issuer.cnf","index.txt","index.txt.attr","serial","crlnumber"}
ROLE_KEYS={"valid","expired","revoked","client"}
MAX_OBJECTS=256
MAX_TOTAL=8*1024*1024
ITERATIONS=json.loads((pathlib.Path(__file__).parent/"toolchain.json").read_text())["calibrated_KDF_candidate"]["iterations"]
DOMAIN=b"SFD-SYNTHETIC-ARCHIVE-AUTH-V3\0"

def canonical(obj):
    return json.dumps(obj,sort_keys=True,separators=(",",":")).encode()

def pem_der(data):
    lines=data.splitlines()
    if not lines or lines[0]!=b"-----BEGIN CERTIFICATE-----" or lines[-1]!=b"-----END CERTIFICATE-----":
        raise Stop("archive-certificate-envelope")
    return base64.b64decode(b"".join(lines[1:-1]),validate=True)

def ledger(root, issuer=None):
    rows={}
    for line in read_small(root/"index.txt").decode("ascii").splitlines():
        p=line.split("\t")
        if len(p)!=6 or p[0] not in ("V","R","E") or not re.fullmatch(r"[A-Fa-f0-9]{1,40}",p[3]):
            raise Stop("archive-ledger-row")
        serial=int(p[3],16)
        if serial<=0 or serial in rows or (p[0]=="R")!=bool(p[2]):
            raise Stop("archive-ledger-identity")
        rows[serial]=p
    certs={}
    rootinfo=certificate(pem_der(read_small(root/"root.pem")))
    folder=root/"newcerts"
    if folder.is_symlink() or not folder.is_dir():raise Stop("archive-issued-directory")
    for path in folder.iterdir():
        if not re.fullmatch("[A-Fa-f0-9]{1,40}.pem",path.name):raise Stop("archive-issued-name")
        c=certificate(pem_der(read_small(path)))
        if c["serial"] in certs:raise Stop("archive-issued-duplicate")
        if c["issuer"]!=rootinfo["subject"]:raise Stop("archive-issued-issuer")
        row=rows.get(c["serial"])
        if row is None or int(datetime.datetime.strptime(row[1],"%y%m%d%H%M%SZ").replace(tzinfo=datetime.timezone.utc).timestamp())!=c["not_after"]:
            raise Stop("archive-issued-expiry")
        if issuer is not None:
            scratch=create_workspace()
            public=child(scratch,"root-public.pem");tbs=child(scratch,"certificate-tbs.bin");sig=child(scratch,"signature.bin")
            parts=seq(pem_der(read_small(path)))
            write_new(public,pem(b"PUBLIC KEY",rootinfo["spki"]))
            write_new(tbs,parts[0][2]);write_new(sig,parts[2][1][1:])
            issuer.command(["dgst","-sha256","-verify",str(public),"-signature",str(sig),str(tbs)])
        certs[c["serial"]]=path.name
    if set(certs)!=set(rows):raise Stop("archive-issued-ledger-reconciliation")
    # Preserve historical V/R/E states exactly; no automatic expiry/status rewrite.
    return rows,certs

def snapshot(issuer,destination,backup_secret):
    src,dst=owned(issuer.root),owned(destination)
    if src==dst:raise Stop("archive-separation")
    if sorted(p.name for p in dst.iterdir())!=["fixture.json"]:raise FileExistsError("archive-destination-not-empty")
    if not isinstance(backup_secret,bytes) or len(backup_secret)<32:raise Stop("archive-secret")
    with publication_lock(src):
        rows,issued=ledger(src,issuer)
        rootder=pem_der(read_small(src/"root.pem"))
        rootinfo=certificate(rootder)
        if b"SyntheticSFDFixture-" not in rootinfo["subject"]:raise Stop("archive-synthetic-only")
        serial_next=int(read_small(src/"serial").strip(),16)
        crl_next=int(read_small(src/"crlnumber").strip(),16)
        if serial_next<=max(rows,default=0):raise Stop("archive-serial-reconciliation")
        generation,history=snapshot_history(src,digest(rootder),digest(rootinfo["spki"]),serial_next,crl_next)
        names=set(CORE)
        names|={p.name for p in src.iterdir() if p.is_file() and p.name.endswith(".csr")}
        for role in ROLE_KEYS:
            for ext in (".pem",".der"):
                if (src/(role+ext)).exists():names.add(role+ext)
        files={name:read_small(child(src,name)) for name in sorted(names)}
        files.update(history)
        for name in issued.values():files["newcerts/"+name]=read_small(src/"newcerts"/name)
        key_names={p.stem for p in src.glob("*.key")}
        if not key_names<=ROLE_KEYS or "valid" not in key_names:raise Stop("archive-server-key-inventory")
        for role in sorted(key_names):
            target=child(dst,"wrapped-"+role+".pem")
            issuer.command(["pkcs8","-topk8","-v2","aes-256-cbc","-v2prf","hmacWithSHA256",
                "-iter",str(ITERATIONS),"-in",str(src/(role+".key")),
                "-passout","fd:SECRET_FD","-out",str(target)],backup_secret)
            os.chmod(target,0o600)
            files["encrypted-server-keys/"+role+".pem"]=read_small(target)
        if len(files)>MAX_OBJECTS or sum(map(len,files.values()))>MAX_TOTAL:raise Stop("archive-size")
        entries={}
        for name,data in sorted(files.items()):
            sha=digest(data);obj=child(dst,"object-"+sha)
            if not obj.exists():write_new(obj,data)
            elif read_small(obj)!=data:raise Stop("archive-object-collision")
            entries[name]={"sha256":sha,"bytes":len(data)}
        m={"schema":SCHEMA,"fixture_only":True,"source_fixture":src.name,
           "root_der_sha256":digest(rootder),"root_spki_sha256":digest(rootinfo["spki"]),
           "generation":generation,"ledger_serials":sorted(rows),"ledger_next_crl":crl_next,"ledger_next_serial":serial_next,
           "files":entries}
        manifest=canonical(m)
        salt=secrets.token_bytes(16)
        header={"schema":SCHEMA,"kdf":"PBKDF2-HMAC-SHA256","iterations":ITERATIONS,
                "salt":salt.hex(),"manifest_sha256":digest(manifest)}
        key=hashlib.pbkdf2_hmac("sha256",backup_secret,DOMAIN+salt,ITERATIONS)
        tag=hmac.new(key,canonical(header)+b"\0"+manifest,"sha256").hexdigest()
        write_new(child(dst,"manifest.json"),manifest)
        write_new(child(dst,"authentication.json"),canonical({**header,"tag":tag}))
        return {"manifest_sha256":digest(manifest),"objects":len(entries),
                "root_der_sha256":m["root_der_sha256"],"server_keys":len(key_names),
                "real_custody_verified":False}

def open_snapshot(root,backup_secret,expected_manifest):
    root=owned(root)
    mbytes=read_small(child(root,"manifest.json"))
    auth=json.loads(read_small(child(root,"authentication.json")))
    expected={"schema","kdf","iterations","salt","manifest_sha256","tag"}
    if set(auth)!=expected or auth["schema"]!=SCHEMA or auth["kdf"]!="PBKDF2-HMAC-SHA256" or auth["iterations"]!=ITERATIONS:
        raise Stop("archive-authentication-profile")
    if not re.fullmatch("[a-f0-9]{32}",auth["salt"]) or not re.fullmatch("[a-f0-9]{64}",auth["tag"]):
        raise Stop("archive-authentication-encoding")
    if digest(mbytes)!=expected_manifest or auth["manifest_sha256"]!=expected_manifest:
        raise Stop("archive-manifest-identity")
    header={k:v for k,v in auth.items() if k!="tag"}
    key=hashlib.pbkdf2_hmac("sha256",backup_secret,DOMAIN+bytes.fromhex(auth["salt"]),ITERATIONS)
    tag=hmac.new(key,canonical(header)+b"\0"+mbytes,"sha256").hexdigest()
    if not hmac.compare_digest(tag,auth["tag"]):raise Stop("archive-authentication-failed")
    m=json.loads(mbytes)
    if m.get("schema")!=SCHEMA or m.get("fixture_only") is not True or canonical(m)!=mbytes:
        raise Stop("archive-schema")
    if not isinstance(m.get("files"),dict) or not 0<len(m["files"])<=MAX_OBJECTS:raise Stop("archive-inventory")
    total=0;contents={}
    for name,item in m["files"].items():
        if name.startswith(PREFIX):
            split_name(name)
        elif (not re.fullmatch(r"(?:newcerts/|encrypted-server-keys/)?[A-Za-z0-9_.-]{1,128}",name)
              or name.endswith(("/..","/.")) or name in (".","..") or event(name)):
            raise Stop("archive-path-or-active-history")
        if set(item)!={"sha256","bytes"} or not re.fullmatch("[a-f0-9]{64}",item["sha256"]) or type(item["bytes"]) is not int:
            raise Stop("archive-object-profile")
        data=read_small(child(root,"object-"+item["sha256"]))
        if len(data)!=item["bytes"] or digest(data)!=item["sha256"]:raise Stop("archive-object-integrity")
        total+=len(data)
        if total>MAX_TOTAL:raise Stop("archive-total-size")
        contents[name]=data
    if not CORE<=contents.keys() or "encrypted-server-keys/valid.pem" not in contents:
        raise Stop("archive-required-object")
    if (type(m.get("ledger_next_serial")) is not int or type(m.get("ledger_next_crl")) is not int
        or m["ledger_next_serial"]<=0 or m["ledger_next_crl"]<=0):
        raise Stop("archive-counter-profile")
    history={n:v for n,v in contents.items() if n.startswith(PREFIX)}
    validate_history(history,m["root_der_sha256"],m["root_spki_sha256"],m["generation"],
                     m["source_fixture"],m["ledger_next_serial"],m["ledger_next_crl"])
    return m,contents

def copy_snapshot(source,target):
    source,target=owned(source),owned(target)
    if source==target:raise Stop("archive-copy-separation")
    for p in source.iterdir():
        if p.name=="fixture.json":continue
        if p.name in ("manifest.json","authentication.json") or re.fullmatch("object-[a-f0-9]{64}",p.name):
            write_new(child(target,p.name),read_small(p))
    return True

def restore(issuer,archive,target,ca_secret,backup_secret,expected_manifest,interrupt_after=None):
    """Authenticate ALL objects before writing a fresh target. Never overwrite or retry."""
    target=owned(target)
    if sorted(p.name for p in target.iterdir())!=["fixture.json"]:raise Stop("archive-restore-target-not-empty")
    m,data=open_snapshot(archive,backup_secret,expected_manifest)
    import uuid
    operation=str(uuid.uuid4())
    write_new(child(target,"restore.started"),canonical({"source_manifest_sha256":expected_manifest,
        "source_fixture":m["source_fixture"],"target_fixture":target.name,
        "generation":m["generation"]+1,"operation":operation,"fixture_only":True}))
    history={n:v for n,v in data.items() if n.startswith(PREFIX)}
    write_history(target,history)
    encrypted={}
    count=0
    for name,content in sorted(data.items()):
        if name.startswith(PREFIX):continue
        if name.startswith("encrypted-server-keys/"):
            encrypted[pathlib.PurePosixPath(name).stem]=content
            continue
        if name.startswith("newcerts/"):
            directory=target/"newcerts"
            if not directory.exists():directory.mkdir(mode=0o700)
            destination=directory/pathlib.PurePosixPath(name).name
        else:destination=child(target,name)
        write_new(destination,content);count+=1
        if interrupt_after is not None and count==interrupt_after:raise Stop("injected-archive-restore-interruption")
    rootder=pem_der(read_small(target/"root.pem"))
    rootinfo=certificate(rootder)
    if digest(rootder)!=m["root_der_sha256"] or digest(rootinfo["spki"])!=m["root_spki_sha256"]:
        raise Stop("archive-restored-root")
    if issuer.recover_identity(target/"ca.enc.pem",ca_secret)!=rootinfo["spki"]:
        raise Stop("archive-restored-CA-identity")
    for role,content in sorted(encrypted.items()):
        if role not in ROLE_KEYS:raise Stop("archive-server-role")
        wrapped=child(target,"restored-encrypted-"+role+".pem")
        write_new(wrapped,content)
        # New disposable server key only; plaintext goes to its protected runtime file, never logs.
        plain=issuer.command(["pkey","-in",str(wrapped),"-passin","fd:SECRET_FD"],backup_secret)
        write_new(child(target,role+".key"),plain)
        pub=issuer.command(["pkey","-in",str(wrapped),"-passin","fd:SECRET_FD","-pubout","-outform","DER"],backup_secret)
        cert=certificate(pem_der(read_small(target/(role+".pem"))))
        if cert["spki"]!=pub:raise Stop("archive-restored-server-identity")
    rows,issued=ledger(target,issuer)
    if (sorted(rows)!=m["ledger_serials"] or int(read_small(target/"crlnumber").strip(),16)!=m["ledger_next_crl"]
        or int(read_small(target/"serial").strip(),16)!=m["ledger_next_serial"]):
        raise Stop("archive-restored-ledger")
    # Public-only RSA/SHA256 signature checks on every retained CRL, including diagnostic profiles.
    scratch=create_workspace()
    public=child(scratch,"root-public.pem")
    write_new(public,pem(b"PUBLIC KEY",rootinfo["spki"]))
    for name,blob in history.items():
        if name.endswith(".crl.der"):
            obj=crl(blob);parts=seq(blob)
            tbs=child(scratch,"tbs-"+digest(blob));sig=child(scratch,"sig-"+digest(blob))
            if not tbs.exists():
                write_new(tbs,parts[0][2]);write_new(sig,parts[2][1][1:])
            issuer.command(["dgst","-sha256","-verify",str(public),"-signature",str(sig),str(tbs)])
            if obj["number"]>=m["ledger_next_crl"]:raise Stop("archive-counter-reconciliation")
            pair=name[:-4]+".pem"
            lines=history.get(pair,b"").splitlines()
            if (not lines or lines[0]!=b"-----BEGIN X509 CRL-----" or lines[-1]!=b"-----END X509 CRL-----" or
                base64.b64decode(b"".join(lines[1:-1]),validate=True)!=blob):
                raise Stop("archive-CRL-envelope-pair")
    result={"schema":SCHEMA,"restored_objects":len(data),"issued_certificates":len(issued),
            "server_keys_restored":len(encrypted),"root_der_sha256":digest(rootder),
            "next_crl_counter":m["ledger_next_crl"],"next_serial":m["ledger_next_serial"],
            "root_spki_sha256":digest(rootinfo["spki"]),"source_manifest_sha256":expected_manifest,
            "source_fixture":m["source_fixture"],"target_fixture":target.name,"generation":m["generation"]+1,
            "operation":operation,"history_sha256":inventory_digest(history),
            "prior_receipt_sha256":digest(history[PREFIX+m["source_fixture"]+"/restore.verified.json"])
                if PREFIX+m["source_fixture"]+"/restore.verified.json" in history else None,
            "CA_and_server_public_identity_verified":True,
            "real_vault_or_distinct_physical_media_verified":False}
    write_new(child(target,"restore.verified.json"),canonical(result))
    return result

def activate_fixture_recovery(root,lease):
    root=owned(root);verify(lease,root)
    receipt=check_current(root)
    if (int(read_small(root/"crlnumber").strip(),16)!=receipt["next_crl_counter"] or
        int(read_small(root/"serial").strip(),16)!=receipt["next_serial"] or
        digest(pem_der(read_small(root/"root.pem")))!=receipt["root_der_sha256"]):
        raise Stop("archive-recovery-authority-state")
    if any((root/n).exists() for n in ("recovery-context.json","recovery-context.used")):
        raise FileExistsError("archive-recovery-context-already-consumed")
    write_new(child(root,"recovery-context.json"),canonical(lease))
    write_new(child(root,"recovery-context.used"),canonical({"schema":"SFD_RECOVERY_AUTHORITY_V1",
        "lease":lease["lease"],"target_fixture":root.name,"operation":receipt["operation"],
        "restore_receipt_sha256":digest(read_small(root/"restore.verified.json"))}))
    return True

def verify_active_recovery(root):
    root=owned(root)
    try:
        receipt=check_current(root)
        lease=json.loads(read_small(root/"recovery-context.json"))
        marker=json.loads(read_small(root/"recovery-context.used"))
    except (FileNotFoundError,KeyError,json.JSONDecodeError) as error:
        raise Stop("archive-recovery-authority-missing") from error
    verify(lease,root)
    expected={"schema":"SFD_RECOVERY_AUTHORITY_V1","lease":lease["lease"],"target_fixture":root.name,
        "operation":receipt["operation"],"restore_receipt_sha256":digest(read_small(root/"restore.verified.json"))}
    if marker!=expected or digest(pem_der(read_small(root/"root.pem")))!=receipt["root_der_sha256"]:
        raise Stop("archive-recovery-authority-binding")
    return True
