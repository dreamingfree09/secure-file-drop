"""Read-only renewal planning from validated synthetic public CRL; no signing/publisher launch."""
from fence import Stop

def renewal_plan(validator,data,ledger_next,now,operation_hours=2):
    info=validator.validate(data,now)
    if type(ledger_next) is not int or ledger_next<=info["number"]:
        raise Stop("renewal-counter")
    if type(operation_hours) is not int or operation_hours<0:
        raise Stop("renewal-budget")
    remaining=info["next_update"]-now
    needed=max(24,operation_hours+6)*3600
    if remaining<needed:raise Stop("renewal-margin")
    return {"CRL_number":info["number"],"ledger_next":ledger_next,
            "renew_by":info["next_update"]-72*3600,
            "warning_at":info["next_update"]-48*3600,
            "critical_at":info["next_update"]-24*3600,
            "expires":info["next_update"],"execution_margin_seconds":needed,
            "signing_enabled":False,"scheduler_installed":False}
