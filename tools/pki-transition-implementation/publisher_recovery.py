"""Forward-only recovery of PUBLIC DISPOSABLE publication snapshots; no live mode."""
import base64,json,time
from fence import Stop,child,digest,owned,read_small,write_new
from context import verify
from publisher_fixture import Publication,encoded,publication_lock

def inspect_candidate(root,validator,bundle_hash,now=None):
    root=owned(root);now=int(time.time()) if now is None else now
    matches=[p for p in root.glob("bundle-*.json") if digest(read_small(p))==bundle_hash]
    if len(matches)!=1:raise Stop("recovery-candidate-missing-or-ambiguous")
    path=child(root,matches[0].name);bundle=json.loads(read_small(path))
    if set(bundle)!={"schema","identity","DER"} or bundle["schema"]!="SFD_PUBLIC_FIXTURE_BUNDLE_V1":
        raise Stop("recovery-bundle-schema")
    data=base64.b64decode(bundle["DER"],validate=True)
    info=validator.validate(data,now)
    if info!=bundle["identity"] or info["next_update"]-now<86400:
        raise Stop("recovery-bundle-binding-margin")
    # Exact immutable filename as originally produced; do not trust a directory sort/newest timestamp.
    if path.name!="bundle-"+str(info["number"])+"-"+bundle_hash+".json":
        raise Stop("recovery-bundle-name")
    return path,info

def reconcile_fixture(root,validator,bundle_hash,reviewed_water,lease,allow_uncommitted=False):
    root=owned(root);verify(lease,root)
    if type(reviewed_water) is not int or reviewed_water<0x2000:
        raise Stop("recovery-reviewed-counter")
    with publication_lock(root):
        path,info=inspect_candidate(root,validator,bundle_hash)
        water=json.loads(read_small(child(root,"highwater.json")))
        if set(water)!={"number"} or water["number"]!=reviewed_water:
            raise Stop("recovery-fresh-water-identity")
        number=info["number"]
        if number<reviewed_water:raise Stop("recovery-no-rollback")
        if number>reviewed_water and allow_uncommitted is not True:
            raise Stop("recovery-uncommitted-not-authorised")
        marker="recovery-"+lease["lease"]+".started"
        write_new(child(root,marker),encoded({"source_SHA256":lease["source_SHA256"],
                  "bundle_sha256":bundle_hash,"water_before":reviewed_water,"target_number":number}))
        publisher=Publication(root,validator,min_number=reviewed_water)
        if number>reviewed_water:
            publisher._replace("highwater.json",encoded({"number":number}))
        # The only repair writes a verified forward/equal pointer to the exact reviewed bundle.
        publisher._replace("current.json",encoded({"bundle":path.name,"sha256":bundle_hash}))
        result=publisher.read()
        if digest(result)!=info["crl_sha256"]:raise Stop("recovery-readback")
        return {"fixture_only":True,"published_number":number,"bundle_sha256":bundle_hash,
                "counter_downgraded":False,"old_pointer_reinstated":False}

def monitor_fixture(publication,now=None):
    now=int(time.time()) if now is None else now
    try:
        data=publication.read(now)
        info=publication.validator.validate(data,now)
        left=info["next_update"]-now
        status="critical" if left<=86400 else "warning" if left<=48*3600 else "renew_due" if left<=72*3600 else "current"
        return {"status":status,"number":info["number"],"next_update":info["next_update"],
                "real_service_started":False,"signing_enabled":False}
    except (ValueError,OSError,KeyError,TypeError):
        return {"status":"STOP","real_service_started":False,"signing_enabled":False}
