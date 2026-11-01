"""Source-bound, one-use synthetic lease. Cannot represent real creation/trust authority."""
import datetime
import json
import pathlib
import uuid
from fence import Stop,child,digest,owned,read_small,write_new
AUTHORITY_ID=6557257514
AUTHORITY_SHA256="3be2056c1ee1eaf23f5a163247826be68a43bb7641bf6a42dedaab767d89d061"
BASE="36685fc23e401a6ef3f549541b602df5f9e608a0"
SOURCE_FILES=tuple(sorted(p.name for p in pathlib.Path(__file__).resolve().parent.iterdir()
                         if p.is_file() and (p.suffix in (".py",".cs",".config") or p.name=="toolchain.json")))

def pins():
    folder=pathlib.Path(__file__).resolve().parent
    return {name:digest((folder/name).read_bytes()) for name in SOURCE_FILES}

def prepare(root,now=None):
    root=owned(root)
    now=now or datetime.datetime.now(datetime.timezone.utc)
    return {"schema":"SFD_SYNTHETIC_PKI_LEASE_V1","operation":"disposable_fixture_only",
            "lease":str(uuid.uuid4()),"fixture":root.name,
            "created":now.isoformat(),"expires":(now+datetime.timedelta(minutes=60)).isoformat(),
            "authority_id":AUTHORITY_ID,"authority_SHA256":AUTHORITY_SHA256,
            "accepted_design":BASE,"source_SHA256":pins(),
            "real_issuer_authority":False,"trust_authority":False,"implementation_accepted":False}

def verify(ctx,root,now=None):
    root=owned(root);now=now or datetime.datetime.now(datetime.timezone.utc)
    expected={"schema","operation","lease","fixture","created","expires","authority_id",
              "authority_SHA256","accepted_design","source_SHA256","real_issuer_authority",
              "trust_authority","implementation_accepted"}
    if set(ctx)!=expected or ctx["schema"]!="SFD_SYNTHETIC_PKI_LEASE_V1" or ctx["operation"]!="disposable_fixture_only":
        raise Stop("synthetic-context-scope")
    if ctx["fixture"]!=root.name or ctx["accepted_design"]!=BASE or ctx["authority_id"]!=AUTHORITY_ID or ctx["authority_SHA256"]!=AUTHORITY_SHA256:
        raise Stop("synthetic-context-authority")
    if any(ctx[k] is not False for k in ("real_issuer_authority","trust_authority","implementation_accepted")):
        raise Stop("synthetic-context-no-real-authority")
    uuid.UUID(ctx["lease"])
    created=datetime.datetime.fromisoformat(ctx["created"]);expires=datetime.datetime.fromisoformat(ctx["expires"])
    if created.tzinfo is None or expires.tzinfo is None or not created<=now<expires or (expires-created).total_seconds()>3600:
        raise Stop("synthetic-context-expired")
    if ctx["source_SHA256"]!=pins():
        raise Stop("synthetic-context-source")
    return True

def bind_once(root):
    ctx=prepare(root);verify(ctx,root)
    write_new(child(root,"source-context.json"),json.dumps(ctx,sort_keys=True).encode())
    write_new(child(root,"source-context.used"),ctx["lease"].encode())
    return ctx
