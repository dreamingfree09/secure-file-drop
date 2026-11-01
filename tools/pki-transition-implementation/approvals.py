"""Future consent previews with phase-specific unknown/known identities; no execution authority."""
import re
from fence import Stop
SCOPES=("create_issuer","install_current_user_root","select_endpoint","retire_old_root")
COMMON=("owner_sid","implementation_sha","aether_comment_sha256","lease_id","expires_utc")
ROOT=("root_sha256","root_spki_sha256","root_thumbprint")
EXTRA={
 "create_issuer":("new_issuer_namespace","key_profile","custody_record_reference","backup_location_reference"),
 "install_current_user_root":("store","store_baseline_sha256"),
 "select_endpoint":("leaf_sha256","CDP","proxy_config_sha256","separate_adoption_review_sha256"),
 "retire_old_root":("store","store_baseline_sha256","replacement_root_sha256","consumer_inventory_sha256","retirement_deadline")
}
def screen(scope,identity):
    if scope not in SCOPES:raise Stop("approval-scope")
    keys=COMMON+ROOT+EXTRA[scope]
    if set(identity)!=set(keys):raise Stop("approval-fields")
    for k,v in identity.items():
        if scope=="create_issuer" and k in ROOT:
            if v is not None:raise Stop("creation-identity-not-created")
            continue
        if not isinstance(v,str) or not v or any(c in v for c in ("\n","\r","\0")):
            raise Stop("approval-value")
        if k.endswith(("_sha256","_sha")) and not re.fullmatch("[0-9a-f]{64}",v):
            raise Stop("approval-hash")
    if not re.fullmatch(r"S-1-[0-9-]+",identity["owner_sid"]):raise Stop("approval-owner")
    if scope!="create_issuer" and not re.fullmatch("[A-F0-9]{40}",identity["root_thumbprint"]):
        raise Stop("approval-thumbprint")
    if "store" in identity and identity["store"]!="CurrentUser/Root":
        raise Stop("approval-trust-scope")
    if scope=="select_endpoint" and not re.fullmatch(r"http://127\.0\.0\.1:18082/sfd-local-ca-[A-Za-z0-9-]+\.crl",identity["CDP"]):
        raise Stop("approval-CDP")
    text={
 "create_issuer":"Future authority: create one independent local CA. The root does not exist yet; DER/SPKI/thumbprint are UNKNOWN. Approve the exact namespace, profile, custody/backup references and reviewed source, then verify actual identities and recovered-credential restore before any trust action. No trust install, publisher or endpoint selection.",
 "install_current_user_root":"Future authority: add exactly this public root to this Owner's CurrentUser/Root. Other applications in this Windows user may trust certificates it signs. No LocalMachine/other store change, endpoint selection or old-root removal.",
 "select_endpoint":"Future authority: select this exact reviewed leaf, CDP and proxy configuration only after strict Windows acceptance and the separately reviewed Docker/backups/recovery/adoption gates. No root-store or database change.",
 "retire_old_root":"Future authority: remove only the old root identified by BOTH DER hash and thumbprint in this Owner's CurrentUser/Root. Require replacement root acceptance and consumer/store inventory closure. Preserve historical files and evidence; no subject-name bulk deletion or old-root reinstall."
 }[scope]
    return "PREPARATION PREVIEW ONLY. No approval or action is enabled.\n"+text+"\n"+"\n".join(
        k+": "+("UNKNOWN — NOT CREATED" if identity[k] is None else identity[k]) for k in keys)

def fixture_approval(scope):
    """Clearly synthetic example; no real future public identity is invented."""
    if scope not in SCOPES:raise Stop("approval-scope")
    values={k:"0"*64 for k in COMMON+ROOT+EXTRA[scope]}
    values.update(owner_sid="S-1-5-21-1",lease_id="SYNTHETIC-EXPIRED-PREVIEW",expires_utc="2000-01-01T00:00:00Z")
    if scope=="create_issuer":
        values.update({k:None for k in ROOT})
        values.update(new_issuer_namespace="SYNTHETIC-NOT-CREATED",key_profile="RSA3072/SHA256 fixture",
                      custody_record_reference="SYNTHETIC memory vault; not real escrow",
                      backup_location_reference="SYNTHETIC temp directory; not distinct physical custody")
    else:values["root_thumbprint"]="A"*40
    if "store" in values:values["store"]="CurrentUser/Root"
    if scope=="select_endpoint":values["CDP"]="http://127.0.0.1:18082/sfd-local-ca-SYNTHETIC.crl"
    if scope=="retire_old_root":values["retirement_deadline"]="2000-01-01T00:00:00Z"
    return values

def approve_real(*_args,**_kwargs):
    raise Stop("REAL_APPROVAL_EXECUTION_NOT_IMPLEMENTED_IN_PREPARATION")
