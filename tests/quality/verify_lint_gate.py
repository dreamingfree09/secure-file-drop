#!/usr/bin/env python3
"""Prove the real CI lint command rejects new findings in never-executed sources.
Only a newly created TemporaryDirectory snapshot is mutated. No application,
configuration, database, container, credential or user file is changed.
"""
import argparse, hashlib, io, json, os, pathlib, re, subprocess, tarfile, tempfile

TOOL = ['go','run','github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0']
FLAGS = ['run','--timeout','5m','--max-issues-per-linter=0','--max-same-issues=0','--uniq-by-line=false']
CONFIG_SHA256 = 'd983dd8f54024e41247d2e92fb51c35af7b07fa0dc97ac655a8f88dde9a6df0a'
APPROVED_ANNOTATIONS = [['internal/server/hash.go', '// #nosec G703 -- Read-only metadata check on the absolute operator-configured executable, never an HTTP path.', 'info, err := os.Stat(toolPath)'], ['internal/server/hash.go', '// #nosec G204 G702 -- Operator-only absolute regular executable; generated local file argument; no shell. Mirrors the reviewed c320861 native boundary.', 'cmd := exec.CommandContext(ctx, toolPath, filePath)'], ['internal/server/quality_regression_test.go', '// #nosec G302 -- Deliberately writable owned fixture; asserts rejection before execution.', 'if err := os.Chmod(file, 0777); err != nil {'], ['tests/e2e/desktop_client_test.go', '// #nosec G204 G702 -- Fixed test executable and validated fresh loopback fixture arguments, no shell.', 'cmd := exec.CommandContext(deadline, "dotnet", dll, base, run, untrusted.URL, "http://"+listener.Addr().String())'], ['tests/e2e/e2e_test.go', '// #nosec G204 G702 -- Test operator selects the Docker CLI; fixed arguments, no shell.', 'c := exec.CommandContext(ctx, cli, args...)'], ['tests/e2e/e2e_test.go', '// #nosec G204 G702 -- Test operator selects the Docker CLI; fixed arguments, no shell.', 'c := exec.CommandContext(cleanCtx, cli, kind, "inspect", "--format", format, name)'], ['tests/e2e/e2e_test.go', '// #nosec G204 G702 -- Test operator selects the CLI; cleanup uses fixed subcommands after label verification.', 'if b, e := exec.CommandContext(cleanCtx, cli, args...).CombinedOutput(); e != nil {'], ['tests/e2e/e2e_test.go', '// #nosec G304 -- f comes only from the two constant legacy fixture names above.', 'b, e := os.ReadFile(filepath.Join("../../internal/db", f))'], ['tests/e2e/e2e_test.go', '// #nosec G204 G702 -- Constant script, verified synthetic loopback target; no shell.', 'cmd := exec.CommandContext(ctx, "node", "../browser/journey.cjs", base)'], ['tests/e2e/e2e_test.go', '// #nosec G124 -- Copy of the HTTP loopback test cookie, deliberately corrupted for rejection.', 'tampered := *alice'], ['tests/e2e/e2e_test.go', '// #nosec G204 G702 -- Parent-verified local Docker CLI, owned container ID, fixed logs subcommand.', 'logsCmd := exec.CommandContext(ctx, cli, "logs", app)'], ['tests/e2e/rustfs_evaluation_test.go', '// #nosec G204 G702 -- Fixed test module, no shell; parent-owned loopback target also checked inside the tool.', 'cmd := exec.CommandContext(ctx, "go", "run", ".", addr, bucket, run)'], ['tests/e2e/rustfs_evaluation_test.go', '// #nosec G306 -- Public upstream SBOM; isolated scanner container needs read access.', 'if err := os.WriteFile(file, body, 0644); err != nil {'], ['tests/e2e/rustfs_evaluation_test.go', '// #nosec G302 -- Public upstream inventory only, fixed t.TempDir path; no private fixture data.', 'if err := os.Chmod(file, 0644); err != nil {'], ['tests/e2e/rustfs_evaluation_test.go', '// #nosec G304 -- Constant basename within t.TempDir; copied only from the owned scanner container.', 'raw, err := os.ReadFile(result)'], ['tests/e2e/rustfs_migration_rehearsal_test.go', '// #nosec G204 G702 -- Fixed browser script; argument is the verified owned loopback port, no shell.', 'cmd := exec.CommandContext(ctx, "node", "tests/browser/journey.cjs", restoredApp.base)'], ['tests/e2e/rustfs_migration_rehearsal_test.go', '// #nosec G304 -- Immutable checkpoint generated in this fixture-owned unique evidence directory.', 'raw, err := os.ReadFile(checkpoint)'], ['tests/e2e/rustfs_migration_rehearsal_test.go', '// #nosec G204 G702 -- Fixed Docker executable and argument lists; no shell.', 'cmd := exec.CommandContext(ctx, f.cli, args...)'], ['tests/e2e/rustfs_migration_rehearsal_test.go', '// #nosec G204 G702 -- Fixed synthetic helper; verified owned container/volume and loopback endpoint, no shell.', 'cmd := exec.CommandContext(f.ctx, "go", "run", ".", s.addr, f.run+"-private", f.run, "generated")'], ['tests/e2e/rustfs_migration_rehearsal_test.go', '// #nosec G304 -- Fixed checkpoint basename in the fixture-owned unique evidence directory.', 'raw, err := os.ReadFile(file)'], ['tests/e2e/storage_evaluation_test.go', '// #nosec G306 -- Synthetic disposable credentials; container uid 1000 needs read access.', 'if err := os.WriteFile(file, b, 0644); err != nil {'], ['tests/e2e/storage_evaluation_test.go', '// #nosec G306 -- Nonsecret test configuration for uid 1000.', 'if err := os.WriteFile(masterFile, []byte("[master.volume_growth]\\ncopy_1 = 1\\ncopy_2 = 1\\ncopy_3 = 1\\ncopy_other = 1\\n"), 0644); err != nil {'], ['tests/e2e/storage_evaluation_test.go', '// #nosec G204 G702 -- Parent-verified local CLI, owned container and fixed logs command; no raw log publication.', 'logs, err := exec.CommandContext(ctx, cli, "logs", id).CombinedOutput()']]
APPROVED_BUILD_TAGS = []


def policy(root, workflow=True):
    config=(root/'.golangci.yml').read_bytes().replace(b'\r\n',b'\n')
    if hashlib.sha256(config).hexdigest()!=CONFIG_SHA256:
        raise ValueError('lint policy changed; explicit reviewed policy/test update required')
    annotations=[];tags=[]
    # CI also checks out a separate reviewed-reference repository. Its Go files
    # are outside this module and must not enter this repository's policy scope.
    if (root/'.git').exists():
        names=subprocess.check_output(['git','ls-files','-z','--','*.go'],cwd=root).decode().split('\0')
        paths=sorted(root/name for name in names if name)
    else:
        # Owned archive snapshots have no Git metadata; include injected probes.
        paths=sorted(root.rglob('*.go'))
    for path in paths:
        lines=path.read_text().splitlines()
        for i,line in enumerate(lines):
            if re.search(r'#nosec|nolint|lint:ignore|lint:file-ignore|gosec:disable|gosec:enable',line):
                statement=next((x.strip() for x in lines[i+1:] if x.strip()),'')
                annotations.append([path.relative_to(root).as_posix(),line.strip(),statement])
            if re.match(r'\s*//(?:go:build| \+build)',line):tags.append([path.relative_to(root).as_posix(),line.strip()])
    if annotations!=APPROVED_ANNOTATIONS:raise ValueError('unreviewed lint suppression change')
    if tags!=APPROVED_BUILD_TAGS:raise ValueError('unreviewed Go build-scope change')
    if workflow:
        text=(root/'.github/workflows/canonical-reconciliation.yml').read_text()
        command=' '.join(TOOL+FLAGS)
        if text.count('          '+command+'\n')!=1 or '          python3 -B tests/quality/verify_lint_gate.py\n' not in text:
            raise ValueError('workflow does not require the tested uncapped gate and policy verifier')
    return True


def lint(root, output):
    env=dict(os.environ,GOTOOLCHAIN='go1.26.9')
    command=TOOL+FLAGS+['--output.json.path',str(output),'--output.text.path',str(output.with_suffix('.txt'))]
    result=subprocess.run(command,cwd=root,env=env,capture_output=True,timeout=360)
    if not output.exists():raise AssertionError('lint did not produce findings evidence')
    findings=json.loads(output.read_text()).get('Issues') or []
    return result.returncode,findings


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--output');args=parser.parse_args()
    repo=pathlib.Path(__file__).resolve().parents[2]
    policy(repo)
    # Require the archived Go source to equal the producer's tested Go source.
    changed=subprocess.check_output(['git','diff','HEAD','--','*.go','.golangci.yml'],cwd=repo)
    if changed.strip():raise AssertionError('commit Go/policy changes before testing this gate')
    archive=subprocess.check_output(['git','archive','HEAD'],cwd=repo)
    checks=[]
    with tempfile.TemporaryDirectory(prefix='sfd-lint-gate-') as directory:
        root=pathlib.Path(directory)/'source';root.mkdir()
        with tarfile.open(fileobj=io.BytesIO(archive)) as source:
            for member in source.getmembers():
                if not (root/member.name).resolve().is_relative_to(root.resolve()):raise AssertionError('unsafe snapshot path')
            source.extractall(root,filter='data')
        policy(root,workflow=False)
        code,findings=lint(root,pathlib.Path(directory)/'clean.json')
        if code!=0 or findings:raise AssertionError('full clean snapshot lint failed')
        checks.append({'scenario':'clean full lint, no baseline filtering','result':'PASS','findings':0})
        probe=root/'internal/server/lint_gate_probe.go'
        if probe.exists():raise AssertionError('probe path already exists')
        probes=[
            ('unused','package server\n\nfunc lintGateUnusedProbe() {}\n','unused',None),
            ('unchecked-error','package server\nimport "io"\n// LintGateErrcheckProbe is never executed.\nfunc LintGateErrcheckProbe(w io.Writer) { w.Write([]byte("synthetic")) }\n','errcheck',None),
            ('security-permissions','package server\nimport "os"\n// LintGateSecurityProbe is never executed.\nfunc LintGateSecurityProbe() error { return os.WriteFile("lint-fixture-never-executed",[]byte("synthetic"),0777) }\n','gosec','G306'),
        ]
        for name,source,linter,rule in probes:
            with probe.open('x') as target:target.write(source)
            try:
                policy(root,workflow=False)
                code,findings=lint(root,pathlib.Path(directory)/(name+'.json'))
                matches=[x for x in findings if x['FromLinter']==linter and (rule is None or x['Text'].startswith(rule+':'))]
                if code==0 or not matches:raise AssertionError('gate failed to reject '+name)
                checks.append({'scenario':'reject new '+name,'result':'PASS','matched_linter':linter,'matched_rule':rule,'nonzero_exit':True})
            finally:
                if probe.read_text()!=source:raise AssertionError('owned probe changed unexpectedly')
                probe.unlink()
        for name,source in [
            ('new suppression','package server\n//nolint:unused\nfunc lintGateSuppressedProbe() {}\n'),
            ('new excluded Go build scope','//go:build ignore\n\npackage server\nfunc lintGateIgnoredProbe() {}\n'),
        ]:
            with probe.open('x') as target:target.write(source)
            try:
                try:policy(root,workflow=False)
                except ValueError:checks.append({'scenario':'reject '+name,'result':'PASS'})
                else:raise AssertionError('policy silently accepted '+name)
            finally:
                if probe.read_text()!=source:raise AssertionError('owned policy probe changed')
                probe.unlink()
        config=root/'.golangci.yml';original=config.read_bytes()
        try:
            config.write_text(original.decode().replace('    - gosec\n',''))
            try:policy(root,workflow=False)
            except ValueError:checks.append({'scenario':'reject disabled security rule/config expansion','result':'PASS'})
            else:raise AssertionError('policy silently accepted disabled gosec')
        finally:config.write_bytes(original)
    result={'work_id':'SFD-LEGACY-LINT-RECONCILIATION-20261019','scenario_count':len(checks),'checks':checks,'probe_sources_executed':False,'owned_snapshot_removed':True,'config_sha256':CONFIG_SHA256,'baseline_filtering':False}
    if args.output:pathlib.Path(args.output).write_text(json.dumps(result,indent=2)+'\n')
    print(json.dumps(result,indent=2))

if __name__=='__main__':main()
