"""Pure fabricated policy fixtures; runner writes only its own synthetic receipt."""
import ast
import hashlib
import importlib.util
import json
import pathlib
import unittest

HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("design_model", HERE / "model.py")
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

def fixture(phase):
    s = {k: "fixture:" + k for k in m.IDENTITIES}
    s.update({k: False for k in m.FORBIDDEN})
    s.update({k: True for k in m.COMMON})
    s.update({k: True for k in m.REQUIRED.get(phase, ())})
    s.update(aki="fixture:authority", ski="fixture:authority",
             fresh_lease=True, lease_minutes=60, store="CurrentUser/Root",
             owner="fixture:owner", signature_valid=True, issuer_matches=True,
             active_leaf_not_revoked=True, now=946684800, this_update=946684800-60,
             next_update=946684800+6*86400, number=0x2000,
             previous_number=0x1fff if phase == "maintenance" else 0x2000,
             cdp="http://127.0.0.1:18082/fixture-new-issuer.crl",
             operation_hours=2, retention_days=30, backup_interval_hours=24,
             file_cleanup_enabled=False)
    return s

class BoundaryTests(unittest.TestCase):
    def accepts(self, phase, **changes):
        s = fixture(phase)
        s.update(changes)
        r = m.assess(phase, s)
        self.assertFalse(r["real_execution_authorized"])
        return r["future_gate_passed"]

    def test_all_future_phase_examples_remain_non_authoritative(self):
        for p in m.PHASES:
            with self.subTest(phase=p):
                self.assertTrue(self.accepts(p))

    def test_every_mandatory_gate_fails_closed_when_absent(self):
        for p in m.PHASES:
            for k in m.COMMON + m.REQUIRED[p]:
                with self.subTest(phase=p, omitted=k):
                    s = fixture(p)
                    del s[k]
                    self.assertFalse(m.assess(p, s)["future_gate_passed"])

    def test_true_string_cannot_supply_an_approval(self):
        self.assertFalse(self.accepts("trust_add", owner_trust_authority="true"))

    def test_unknown_phase(self):
        self.assertFalse(self.accepts("start_docker"))

    def test_nondictionary_scenario(self):
        self.assertFalse(m.assess("design", None)["future_gate_passed"])

    def test_real_hash_cannot_masquerade_as_fixture(self):
        self.assertFalse(self.accepts("verify", root="0"*64))

    def test_no_missing_identity(self):
        self.assertFalse(self.accepts("verify", leaf=None))

    def test_all_prohibited_actions_refused(self):
        for p in m.PHASES:
            for key in m.FORBIDDEN:
                with self.subTest(phase=p, operation=key):
                    self.assertFalse(self.accepts(p, **{key:True}))

    def test_no_old_issuer_reuse(self):
        self.assertFalse(self.accepts("preview", issuer="fixture:old_issuer"))

    def test_invalid_signature(self):
        self.assertFalse(self.accepts("preview", signature_valid=False))

    def test_wrong_issuer(self):
        self.assertFalse(self.accepts("preview", issuer_matches=False))

    def test_wrong_aki(self):
        self.assertFalse(self.accepts("preview", aki="fixture:wrong"))

    def test_active_leaf_revoked(self):
        self.assertFalse(self.accepts("verify", active_leaf_not_revoked=False))

    def test_expired_crl(self):
        self.assertFalse(self.accepts("verify", next_update=946684800))

    def test_not_yet_valid_crl(self):
        self.assertFalse(self.accepts("verify", this_update=946684801))

    def test_crl_lifetime_excess(self):
        self.assertFalse(self.accepts("verify", next_update=946684800+8*86400))

    def test_unknown_validity_type(self):
        self.assertFalse(self.accepts("verify", next_update="fixture:tomorrow"))

    def test_boolean_cannot_supply_counter(self):
        self.assertFalse(self.accepts("maintenance", number=True))

    def test_publish_must_advance_number(self):
        self.assertFalse(self.accepts("maintenance", previous_number=0x2000))

    def test_publish_cannot_downgrade_number(self):
        self.assertFalse(self.accepts("maintenance", previous_number=0x2001))

    def test_readback_number_must_match_publication(self):
        self.assertFalse(self.accepts("verify", previous_number=0x2001))

    def test_old_crl_sequence_refused(self):
        self.assertFalse(self.accepts("verify", number=0x1001, previous_number=0x1001))

    def test_old_cdp_refused(self):
        self.assertFalse(self.accepts("verify", cdp="http://127.0.0.1:18081/sfd-local-ca.crl"))

    def test_remote_cdp_refused(self):
        self.assertFalse(self.accepts("verify", cdp="http://192.0.2.1/crl"))

    def test_short_operation_still_requires_24_hours(self):
        self.assertFalse(self.accepts("selection", next_update=946684800+23*3600))

    def test_long_operation_requires_budget_plus_six(self):
        self.assertFalse(self.accepts("selection", operation_hours=30, next_update=946684800+35*3600))

    def test_exact_margin_boundary(self):
        self.assertTrue(self.accepts("selection", operation_hours=30, next_update=946684800+36*3600))

    def test_negative_budget(self):
        self.assertFalse(self.accepts("selection", operation_hours=-1))

    def test_missing_lease(self):
        self.assertFalse(self.accepts("creation", fresh_lease=False))

    def test_overlong_lease(self):
        self.assertFalse(self.accepts("creation", lease_minutes=61))

    def test_zero_lease(self):
        self.assertFalse(self.accepts("creation", lease_minutes=0))

    def test_machine_trust_addition_forbidden(self):
        self.assertFalse(self.accepts("trust_add", store="LocalMachine/Root"))

    def test_non_root_store_forbidden(self):
        self.assertFalse(self.accepts("trust_add", store="CurrentUser/My"))

    def test_other_user_trust_forbidden(self):
        self.assertFalse(self.accepts("trust_add", owner="fixture:other-owner"))

    def test_retirement_requires_exact_issuer_identity(self):
        self.assertFalse(self.accepts("retirement", exact_old_der_and_thumb=False))

    def test_retirement_unknown_consumer(self):
        self.assertFalse(self.accepts("retirement", consumers_closed=False))

    def test_pretrust_dotnet_must_reject(self):
        self.assertFalse(self.accepts("preview", untrusted_root_rejected=False))

    def test_creation_custody_precondition(self):
        self.assertFalse(self.accepts("creation", custody_accessible=False))

    def test_postcreation_recovery_test_required(self):
        self.assertFalse(self.accepts("trust_add", recovered_secret_verified=False))

    def test_valid_cached_crl_does_not_replace_current_der_readback(self):
        self.assertFalse(self.accepts("verify", current_der_readback=False, cached_valid_crl=True))

    def test_valid_pki_does_not_clear_disk_gate(self):
        self.assertFalse(self.accepts("adoption", cold_start_gate=False))

    def test_no_backend_selection_with_unknown_backups(self):
        self.assertFalse(self.accepts("selection", backup_gate=False))

    def test_no_seven_day_retention_substitution(self):
        self.assertFalse(self.accepts("selection", retention_days=7))

    def test_backup_schedule_preserved(self):
        self.assertFalse(self.accepts("adoption", backup_interval_hours=48))

    def test_file_cleanup_stays_disabled(self):
        self.assertFalse(self.accepts("adoption", file_cleanup_enabled=True))

    def test_renewal_alert_boundaries(self):
        for hours, status in ((73,"current"),(72,"renew_due"),(48,"warning"),(24,"critical"),(0,"expired"),(-1,"expired")):
            with self.subTest(hours=hours):
                self.assertEqual(m.renewal_status(0, hours*3600), status)

    def test_future_real_identities_are_unset(self):
        p = json.loads((HERE/"policy.json").read_text())
        self.assertTrue(all(v is None for v in p["future_real_identity"].values()))
        self.assertFalse(p["real_execution_authorized"])
        self.assertEqual(p["mode"], "design_only")

    def test_model_is_inert(self):
        tree = ast.parse((HERE/"model.py").read_text())
        forbidden = {"open","exec","eval","compile","__import__","getattr","setattr","globals","locals"}
        self.assertFalse(any(isinstance(n,(ast.Import,ast.ImportFrom)) for n in ast.walk(tree)))
        self.assertFalse(any(isinstance(n,ast.Call) and isinstance(n.func,ast.Name) and n.func.id in forbidden for n in ast.walk(tree)))
        self.assertFalse(any(isinstance(n,ast.Name) and n.id.startswith("__") for n in ast.walk(tree)))

if __name__ == "__main__":
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(BoundaryTests)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    report = {
        "scope": "inert synthetic policy only; no cryptographic fixtures or real operations",
        "mode": "synthetic_policy_simulation",
        "tests_run": result.testsRun,
        "failures": len(result.failures), "errors": len(result.errors),
        "skips": len(result.skipped),
        "test_names": unittest.defaultTestLoader.getTestCaseNames(BoundaryTests),
        "source_sha256": {n:hashlib.sha256((HERE/n).read_bytes()).hexdigest()
                          for n in ("model.py","test_model.py","policy.json")},
        "network_requests":0, "private_key_checks":0, "generated_crypto_artifacts":0,
        "real_TLS_acceptance_executed":False, "trust_changes":0,
        "Docker_operations":0, "real_execution_authorized":False
    }
    (HERE/"synthetic-results.json").write_text(json.dumps(report,indent=2)+"\n")
    raise SystemExit(0 if result.wasSuccessful() else 1)
