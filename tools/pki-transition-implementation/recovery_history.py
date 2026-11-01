"""Versioned authenticated fixture history. Never a source of active signing authority."""
import json
import os
import pathlib
import re
import uuid
from fence import Stop, owned, read_small, write_new, digest

PREFIX="history/v1/"
PROVENANCE="provenance.json"
SCHEMA="SFD_SYNTHETIC_RECOVERY_HISTORY_V1"

def canonical(obj):
    return json.dumps(obj,sort_keys=True,separators=(",",":")).encode()

def event(name):
    return (name=="fixture.json" or name.endswith((".started",".used",".crl.der",".crl.pem"))
            or name in ("source-context.json","recovery-context.json","restore.verified.json",
                        "index.txt.old","index.txt.attr.old","serial.old","crlnumber.old")
            or bool(re.fullmatch(r"diagnostic-[0-9a-f-]{36}.json",name)))

def split_name(name):
    m=re.fullmatch(r"history/v1/(sfd-pki-fixtures-([0-9a-f-]{36}))/([A-Za-z0-9_.-]{1,128})",name)
    if not m or str(uuid.UUID(m[2]))!=m[2] or m[3] in (".",".."):
        raise Stop("history-path")
    if m[3]!=PROVENANCE and not event(m[3]):raise Stop("history-event")
    return m[1],m[3]

def inventory_digest(data):
    return digest(canonical({n:{"sha256":digest(v),"bytes":len(v)} for n,v in sorted(data.items())}))

def read_history(root):
    root=owned(root);base=root/"history";result={}
    if base.is_symlink():raise Stop("history-reparse")
    if not base.exists():return result
    def walk(path):
        if path.is_symlink():raise Stop("history-reparse")
        if path.is_dir():
            if path!=base and not (path==base/"v1" or path.parent==base/"v1"):
                raise Stop("history-directory")
            for p in path.iterdir():walk(p)
        else:
            name=path.relative_to(root).as_posix();split_name(name);result[name]=read_small(path)
            if len(result)>256 or sum(map(len,result.values()))>8*1024*1024:raise Stop("history-bound")
    walk(base)
    return result

def _validate(data,root_sha,spki_sha,generation,source_fixture,serial_next,crl_next):
    if type(generation) is not int or not 0<=generation<256:raise Stop("history-generation")
    groups={}
    for name,value in data.items():
        fixture,short=split_name(name);groups.setdefault(fixture,{})[short]=value
    ordered=[]
    for fixture,entries in groups.items():
        if PROVENANCE not in entries:raise Stop("history-provenance-missing")
        p=json.loads(entries[PROVENANCE])
        fields={"schema","fixture","generation","root_der_sha256","root_spki_sha256","serial_next",
                "crl_next","parent_manifest_sha256","files"}
        if set(p)!=fields or p["schema"]!=SCHEMA or canonical(p)!=entries[PROVENANCE] or p["fixture"]!=fixture:
            raise Stop("history-provenance-profile")
        if p["root_der_sha256"]!=root_sha or p["root_spki_sha256"]!=spki_sha:
            raise Stop("history-issuer")
        if type(p["generation"]) is not int or type(p["serial_next"]) is not int or type(p["crl_next"]) is not int:
            raise Stop("history-counter-type")
        actual={n:{"sha256":digest(v),"bytes":len(v)} for n,v in entries.items() if n!=PROVENANCE}
        if p["files"]!=actual:raise Stop("history-event-integrity")
        marker=json.loads(entries["fixture.json"])
        if marker!={"schema":"SFD_DISPOSABLE_PKI_FIXTURE_V1","nonce":fixture[-36:],"synthetic_only":True}:
            raise Stop("history-fixture")
        ordered.append((p["generation"],fixture,p,entries))
    ordered.sort()
    if [x[0] for x in ordered]!=list(range(generation+1)):raise Stop("history-generation-chain")
    prior={}
    for i,fixture,p,entries in ordered:
        if i==0:
            if p["parent_manifest_sha256"] is not None or "restore.verified.json" in entries:
                raise Stop("history-origin")
        else:
            previous=ordered[i-1]
            if p["serial_next"]<previous[2]["serial_next"] or p["crl_next"]<previous[2]["crl_next"]:
                raise Stop("history-counter-regression")
            r=json.loads(entries["restore.verified.json"]);started=json.loads(entries["restore.started"])
            if (r["generation"]!=i or r["target_fixture"]!=fixture or
                r["source_fixture"]!=previous[1] or r["source_manifest_sha256"]!=p["parent_manifest_sha256"] or
                r["history_sha256"]!=inventory_digest(prior) or
                r["prior_receipt_sha256"]!=(digest(previous[3]["restore.verified.json"]) if i>1 else None) or
                r["next_crl_counter"]!=previous[2]["crl_next"] or r["next_serial"]!=previous[2]["serial_next"] or
                r["root_der_sha256"]!=root_sha or r["root_spki_sha256"]!=spki_sha or
                started["operation"]!=r["operation"] or started["target_fixture"]!=fixture or
                started["source_manifest_sha256"]!=r["source_manifest_sha256"]):
                raise Stop("history-receipt-chain")
        prior.update({PREFIX+fixture+"/"+n:v for n,v in entries.items()})
    last=ordered[-1]
    if last[1]!=source_fixture or last[2]["serial_next"]!=serial_next or last[2]["crl_next"]!=crl_next:
        raise Stop("history-current-state")
    return last[2]

def validate(*args):
    try:return _validate(*args)
    except (KeyError,TypeError,IndexError,json.JSONDecodeError) as error:
        raise Stop("history-invalid-provenance") from error


def snapshot_history(root,root_sha,spki_sha,serial_next,crl_next):
    root=owned(root);history=read_history(root)
    receipt_path=root/"restore.verified.json"
    if receipt_path.exists():
        receipt=check_current(root)
        generation=receipt["generation"];parent=receipt["source_manifest_sha256"]
    else:
        if history or (root/"restore.started").exists():raise Stop("history-unverified-source")
        generation=0;parent=None
    entries={p.name:read_small(p) for p in root.iterdir() if event(p.name)}
    if any(n.startswith(PREFIX+root.name+"/") for n in history):raise Stop("history-namespace-collision")
    provenance={"schema":SCHEMA,"fixture":root.name,"generation":generation,"root_der_sha256":root_sha,
                "root_spki_sha256":spki_sha,"serial_next":serial_next,"crl_next":crl_next,
                "parent_manifest_sha256":parent,
                "files":{n:{"sha256":digest(v),"bytes":len(v)} for n,v in entries.items()}}
    entries[PROVENANCE]=canonical(provenance)
    history.update({PREFIX+root.name+"/"+n:v for n,v in entries.items()})
    validate(history,root_sha,spki_sha,generation,root.name,serial_next,crl_next)
    return generation,history

def write_history(root,data):
    root=owned(root)
    for name,value in sorted(data.items()):
        split_name(name);path=root/name
        path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
        write_new(path,value);os.chmod(path,0o400)
    # Read-only archive-only evidence, exclusive writes; not protection from a compromised owner.
    for folder in sorted((root/"history").rglob("*"),key=lambda p:len(p.parts),reverse=True):
        if folder.is_dir():os.chmod(folder,0o500)
    os.chmod(root/"history",0o500)

def check_current(root):
    root=owned(root);r=json.loads(read_small(root/"restore.verified.json"));s=json.loads(read_small(root/"restore.started"))
    history=read_history(root)
    if (r["target_fixture"]!=root.name or r["operation"]!=s["operation"] or
        s["target_fixture"]!=root.name or s["source_manifest_sha256"]!=r["source_manifest_sha256"] or
        r["generation"]!=s["generation"] or r["source_fixture"]!=s["source_fixture"] or
        r["history_sha256"]!=inventory_digest(history) or
        r["CA_and_server_public_identity_verified"] is not True):
        raise Stop("history-active-receipt")
    uuid.UUID(r["operation"])
    validate(history,r["root_der_sha256"],r["root_spki_sha256"],r["generation"]-1,
             r["source_fixture"],r["next_serial"],r["next_crl_counter"])
    latest=PREFIX+r["source_fixture"]+"/restore.verified.json"
    if r["prior_receipt_sha256"]!=(digest(history[latest]) if latest in history else None):
        raise Stop("history-active-prior-receipt")
    if (root/"source-context.json").exists() or (root/"source-context.used").exists():
        raise Stop("history-no-source-lease-on-restored-target")
    if int(read_small(root/"serial").strip(),16)<r["next_serial"] or int(read_small(root/"crlnumber").strip(),16)<r["next_crl_counter"]:
        raise Stop("history-active-counter-regression")
    return r
