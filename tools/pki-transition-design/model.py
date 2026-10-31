"""Inert policy model. Fabricated values only; no crypto, I/O or execution authority."""
PHASES = ("design", "creation", "preview", "trust_add", "verify", "selection",
          "retirement", "maintenance", "adoption")
IDENTITIES = ("issuer", "old_issuer", "root", "leaf", "crl", "aki", "ski")
COMMON = ("preservation", "exact_review")
REQUIRED = {
    "design": (),
    "creation": ("owner_creation_authority", "custody_accessible", "backup_destination",
                 "private_acl", "exclusive_namespace", "hidden_local_input"),
    "preview": ("custody_restore", "recovered_secret_verified", "public_profile",
                "untrusted_root_rejected", "isolated_fixture_authority"),
    "trust_add": ("owner_trust_authority", "custody_restore", "recovered_secret_verified",
                  "public_profile", "untrusted_root_rejected", "fresh_store_baseline",
                  "account_wide_effect_disclosed"),
    "verify": ("root_trusted_in_approved_scope", "strict_schannel", "strict_framework",
               "revocation_enabled", "current_der_readback", "served_leaf_matches",
               "negative_matrix", "reviewed_windows_bundle"),
    "selection": ("owner_selection_authority", "strict_suite", "reviewed_windows_bundle",
                  "cold_start_gate", "backup_gate", "restore_gate", "idle_gate",
                  "image_gate", "recovery_gate"),
    "retirement": ("owner_removal_authority", "strict_suite", "consumers_closed",
                   "no_old_leaf_served", "exact_old_der_and_thumb", "fresh_store_baseline",
                   "retirement_deadline_recorded"),
    "maintenance": ("renewal_authority", "recovered_secret_verified", "counter_bound",
                    "one_attempt", "immutable_outputs", "publisher_verified"),
    "adoption": ("strict_suite", "cold_start_gate", "backup_gate", "restore_gate",
                 "idle_gate", "image_gate", "recovery_gate", "schema_gate",
                 "real_auth_gate", "owner_isolation_gate", "storage_gate")
}
FORBIDDEN = ("copy_old_key", "retry_spent_attempt", "counter_rollback", "old_crl_fallback",
             "disable_revocation", "real_execution_requested", "global_cache_flush",
             "global_clock_change", "modify_old_publication", "unrelated_store_edit")

def crl_errors(s, *, publishing=False, require_margin=False):
    """All signatures and identities below are synthetic assertions, not verification."""
    errors = []
    for key in ("signature_valid", "issuer_matches", "active_leaf_not_revoked"):
        if s.get(key) is not True:
            errors.append(key)
    if s.get("aki") != s.get("ski"):
        errors.append("aki_mismatch")
    if s.get("issuer") == s.get("old_issuer"):
        errors.append("old_issuer_reuse")
    vals = [s.get(x) for x in ("now", "this_update", "next_update", "number", "previous_number")]
    if any(type(x) is not int for x in vals):
        return errors + ["invalid_time_or_counter_type"]
    now, start, end, number, previous = vals
    if not start <= now < end or end - start > 7 * 86400:
        errors.append("crl_validity")
    if number < 0x2000:
        errors.append("independent_sequence")
    if publishing and number <= previous:
        errors.append("nonincreasing_publication")
    if not publishing and number != previous:
        errors.append("unbound_publication")
    if s.get("cdp") != "http://127.0.0.1:18082/fixture-new-issuer.crl":
        errors.append("cdp")
    if require_margin:
        budget = s.get("operation_hours")
        if type(budget) is not int or budget < 0:
            errors.append("operation_budget")
        elif end - now < max(24, budget + 6) * 3600:
            errors.append("insufficient_margin")
    return errors

def renewal_status(now, next_update):
    left = next_update - now
    if left <= 0:
        return "expired"
    if left <= 24 * 3600:
        return "critical"
    if left <= 48 * 3600:
        return "warning"
    if left <= 72 * 3600:
        return "renew_due"
    return "current"

def assess(phase, s):
    errors = []
    if phase not in PHASES:
        errors.append("unknown_phase")
    if not isinstance(s, dict):
        return {"mode": "synthetic_policy_simulation", "future_gate_passed": False,
                "real_execution_authorized": False, "errors": ["scenario_type"]}
    for key in IDENTITIES:
        if not isinstance(s.get(key), str) or not s[key].startswith("fixture:"):
            errors.append("nonfixture_identity:" + key)
    for key in FORBIDDEN:
        if s.get(key) is not False:
            errors.append("forbidden_or_unspecified:" + key)
    for key in COMMON + REQUIRED.get(phase, ()):
        if s.get(key) is not True:
            errors.append("missing:" + key)
    if phase in ("trust_add", "retirement"):
        if s.get("store") != "CurrentUser/Root" or s.get("owner") != "fixture:owner":
            errors.append("trust_scope")
    if phase not in ("design",):
        if s.get("fresh_lease") is not True or type(s.get("lease_minutes")) is not int or not 0 < s["lease_minutes"] <= 60:
            errors.append("lease")
    if phase in ("preview", "trust_add", "verify", "selection", "maintenance", "adoption", "retirement"):
        errors += crl_errors(s, publishing=phase == "maintenance",
                             require_margin=phase in ("selection", "maintenance", "adoption"))
    if phase in ("selection", "adoption") and s.get("retention_days") != 30:
        errors.append("retention")
    if phase in ("selection", "adoption") and (s.get("backup_interval_hours") != 24 or s.get("file_cleanup_enabled") is not False):
        errors.append("schedule_or_cleanup")
    return {"mode": "synthetic_policy_simulation", "future_gate_passed": not errors,
            "real_execution_authorized": False, "errors": sorted(set(errors))}
