"""Pure, fail-closed policy evaluation. Reports never authorise a mutation.

Offline fixture input is explicitly non-authoritative. The Windows installer
recollects live evidence and validates remote exact-source approvals itself.
No Docker, WSL, VM, network, certificate or original-file operation lives here.
"""
import argparse
import datetime as dt
import hashlib
import math
import json
import pathlib
import re
import sys
import ntpath

HERE = pathlib.Path(__file__).resolve().parent
POLICY = json.loads((HERE / 'policy.json').read_text(encoding='utf-8'))

class Stop(Exception):
    pass

def strict_json(raw):
    def unique(pairs):
        value = {}
        for key, item in pairs:
            if key in value:
                raise Stop('duplicate-json-key')
            value[key] = item
        return value
    try:
        return json.loads(raw, object_pairs_hook=unique,
                          parse_constant=lambda _: (_ for _ in ()).throw(Stop('nonfinite-json')))
    except (ValueError, TypeError) as e:
        raise Stop('invalid-json') from e

def iso(value):
    try:
        t = dt.datetime.fromisoformat(value.replace('Z', '+00:00'))
        if t.utcoffset() != dt.timedelta(0):
            raise Stop('utc-required')
        return t
    except (ValueError, AttributeError) as e:
        raise Stop('invalid-time') from e

def inside(candidate):
    """Lexical check only. Native collector must also verify volume/reparse/file IDs."""
    if not isinstance(candidate, str) or any(c in candidate for c in ('\0', '\n', '\r')):
        return False
    root = POLICY['root']
    if '/' in candidate or candidate.startswith('\\\\') or ':' in candidate[2:]:
        return False
    if any(x in ('.', '..') or x.endswith((' ', '.')) for x in candidate.split('\\')):
        return False
    return ntpath.normcase(candidate).startswith(ntpath.normcase(root + '\\'))

def validate_packet(*args,**kwargs):
    # V1 used one account for both roles and is permanently retired.
    raise Stop('legacy-approval-retired')

def validate_approvals(review,owner,context,now):
    from approval import validate_pair
    return validate_pair(review,owner,context,now)


def evaluate(facts, phase='admit'):
    """Every absent/unknown/wrongly typed fact is a rejection, never a default pass."""
    failures = []
    def require(code, predicate):
        if predicate is not True:
            failures.append(code)
    def flag(name):
        return facts.get(name) is True
    require('schema', facts.get('schema') == 'SFD_STAGE1_LIVE_FACTS_V1')
    require('phase', phase in ('protect', 'admit'))
    require('collector', facts.get('collector') == 'collect-host.ps1')
    require('no-protected-byte-access', flag('no_protected_byte_access'))
    for field in ['source_clean','source_manifest_verified','authority_exact','owner_go_exact',
                  'host_admin','HyperV_capable','host_RAM_available','host_commit_margin','host_identity_bound','maintenance_window','no_pending_reboot',
                  'H_identity_bound','H_ntfs','H_root_absent_or_owned','no_reparse',
                  'no_broad_lab_write','no_Docker_processes','no_WSL_workloads',
                  'HCS_query_complete','no_unknown_compute_systems','disk_inventory_complete',
                  'no_file_backed_unknown_disks','no_original_image_attachments',
                  'media_verified','media_rights_verified','runtime_bundle_verified']:
        require(field, flag(field))
    for field, minimum in [('C_free_GiB', POLICY['min_C_free_GiB'] if phase=='protect' else POLICY['min_C_run_free_GiB']),
                           ('H_free_GiB', POLICY['min_H_free_GiB'] if phase=='protect' else POLICY['min_H_run_free_GiB']),
                           ('RAM_GiB', POLICY['host_RAM_min_GiB'])]:
        value = facts.get(field)
        require(field, type(value) in (int,float) and math.isfinite(value) and value >= minimum)
    require('freshness', type(facts.get('age_seconds')) in (int,float)
            and 0 <= facts['age_seconds'] <= 30)
    if phase == 'protect':
        # Protection is itself a reserved, approval-bound host change. It cannot
        # run while original workloads or preexisting workloads need disruption.
        require('immutable-service-baseline', flag('service_snapshot_immutable'))
        require('original-Docker-already-stopped', flag('Docker_service_stopped'))
        require('protection-rollback-snapshot', flag('rollback_snapshot_ready'))
    else:
        for field in ['preventive_services_disabled_stopped','exclusive_activation_locks_verified',
                      'guardian_exact_binary_identities','native_lock_canary_verified',
                      'guardian_challenge_verified','HyperV_ready','fixed_disks_reserved',
                      'private_switch_verified','no_management_OS_adapter',
                      'two_guest_ids_bound','all_disks_new_owned','no_extra_guest_NICs',
                      'no_host_shares','no_default_gateway_DNS','no_external_route',
                      'guest_payload_verified','vm_autostart_nothing','checkpoint_off',
                      'secure_boot_vTPM_verified','H_recovery_reservation_verified','independent_supervisor_verified']:
            require(field, flag(field))
    return {'schema':'SFD_STAGE1_GUARD_RESULT_V1', 'phase':phase,
            'decision':'PASS_EVIDENCE' if not failures else 'NO-GO',
            'failures':failures, 'authorises_mutation':False,
            'note':'Evidence evaluation only; not an Owner GO or an execution receipt.'}

def verify_manifest(root):
    root = pathlib.Path(root).resolve(strict=True)
    manifest = strict_json((root/'manifest.json').read_text(encoding='utf-8'))
    if manifest.get('schema') != 'SFD_STAGE1_SOURCE_MANIFEST_V1':
        raise Stop('manifest-schema')
    seen = set()
    for row in manifest['files']:
        rel = row['path']
        parts = pathlib.PurePosixPath(rel).parts
        if not parts or rel in seen or any(p in ('.','..') or ':' in p for p in parts) or rel.startswith('/'):
            raise Stop('manifest-path')
        seen.add(rel)
        target = root.joinpath(*parts)
        # Refuse symlinks/junction-like paths, not just final-file aliases.
        for parent in [target, *list(target.parents)[:len(parts)-1]]:
            stat = parent.lstat()
            if parent.is_symlink() or getattr(stat, 'st_file_attributes', 0) & 0x400:
                raise Stop('manifest-reparse')
        raw = target.read_bytes()
        if len(raw) != row['bytes'] or hashlib.sha256(raw).hexdigest() != row['SHA256']:
            raise Stop('manifest-identity')
    return hashlib.sha256((root/'manifest.json').read_bytes()).hexdigest()

def main():
    p = argparse.ArgumentParser()
    p.add_argument('--facts', type=pathlib.Path, required=True)
    p.add_argument('--phase', choices=['protect','admit'], default='admit')
    p.add_argument('--fixture', action='store_true')
    args = p.parse_args()
    try:
        result = evaluate(strict_json(args.facts.read_text(encoding='utf-8-sig')),args.phase)
        result['synthetic_fixture'] = args.fixture
        print(json.dumps(result))
        return 0 if result['decision']=='PASS_EVIDENCE' else 2
    except (Stop,OSError,KeyError,TypeError) as e:
        print(json.dumps({'decision':'NO-GO','code':str(e) if isinstance(e,Stop) else type(e).__name__,
                          'authorises_mutation':False}))
        return 2
if __name__ == '__main__':
    sys.exit(main())
