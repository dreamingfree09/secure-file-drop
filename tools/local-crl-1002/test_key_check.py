#!/usr/bin/env python3
"""Synthetic PTY regression tests; never references an Owner CA key or real password."""
import datetime as dt
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import pty
import select
import signal
import subprocess
import termios
import time

HELPER=Path(__file__).with_name("owner-check-ca-key.py")
OPENSSL=Path("/usr/bin/openssl")
FIXTURE_PASSWORD=b"synthetic-only-fixture-5021"
ROOT=Path("/home/dreamingfree09/.local/share/secure-file-drop/key-check-tests/synthetic-20261030-"+dt.datetime.now(dt.timezone.utc).strftime("%H%M%S")+"-"+os.urandom(4).hex())
def sha(data):return hashlib.sha256(data).hexdigest()
spec=importlib.util.spec_from_file_location("non_signing_key_check",HELPER)
mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod)
def main():
    os.umask(0o077)
    ROOT.mkdir(parents=True,exist_ok=False,mode=0o700)
    key=ROOT/"synthetic.key"
    subprocess.run([str(OPENSSL),"genpkey","-algorithm","RSA","-pkeyopt","rsa_keygen_bits:2048",
                    "-aes-256-cbc","-pass","stdin","-out",str(key)],
                   input=FIXTURE_PASSWORD+b"\n",capture_output=True,check=True,timeout=20)
    pub=subprocess.run([str(OPENSSL),"pkey","-in",str(key),"-pubout","-outform","DER","-passin","stdin"],
                       input=FIXTURE_PASSWORD+b"\n",capture_output=True,check=True,timeout=10).stdout
    results=[]
    def context(name,wrong_identity=False,expired=False):
        out=ROOT/name;out.mkdir(mode=0o700)
        marker=out/"synthetic-counter";marker.write_text("1002\n")
        ctx=dict(kind="SFD_CA_UNLOCK_CHECK_V1",signing_authorized=False,max_attempts=3,
                 helper_sha256=sha(HELPER.read_bytes()),output_dir=str(out),
                 valid_until=(dt.datetime.now(dt.timezone.utc)+dt.timedelta(minutes=-1 if expired else 5)).isoformat(),
                 key_path=str(key),key_metadata=mod.key_metadata(key),
                 pins=[dict(path=str(marker),sha256=sha(marker.read_bytes()))],
                 openssl=str(OPENSSL),openssl_sha256=sha(OPENSSL.read_bytes()),
                 expected_public_spki_sha256=("0"*64 if wrong_identity else sha(pub)))
        data=json.dumps(ctx).encode();path=out/"context.json";path.write_bytes(data);path.chmod(0o400)
        return out,ctx,[str("/usr/bin/python3"),"-I","-B",str(HELPER),"--context",str(path),"--context-sha256",sha(data)]
    def terminal_case(name,dialogue,expected_success,expected_attempts,stop_code=None,wrong_identity=False,expired=False,mutate=False):
        out,ctx,args=context(name,wrong_identity,expired)
        if mutate:Path(ctx["pins"][0]["path"]).write_text("CHANGED\n")
        before=mod.key_metadata(key)
        master,slave=pty.openpty()
        def child_setup():
            os.setsid();fcntl.ioctl(slave,termios.TIOCSCTTY,0)
        proc=subprocess.Popen(args,stdin=slave,stdout=slave,stderr=slave,preexec_fn=child_setup,close_fds=True)
        os.close(slave);data=b"";position=0;event=0;end=time.monotonic()+25
        try:
            while proc.poll() is None:
                if time.monotonic()>end:raise RuntimeError("SYNTHETIC_PTY_TIMEOUT")
                if select.select([master],[],[],.1)[0]:
                    try:chunk=os.read(master,16384)
                    except OSError:break
                    data+=chunk
                    while event<len(dialogue):
                        needle,reply=dialogue[event];index=data.find(needle,position)
                        if index<0:break
                        os.write(master,reply+b"\n");position=index+len(needle);event+=1
            rc=proc.wait(timeout=3)
        except BaseException:
            if proc.poll() is None:os.killpg(proc.pid,signal.SIGKILL);proc.wait()
            raise
        finally:os.close(master)
        r=json.loads((out/"result.json").read_text())
        assert r["success"] is expected_success and r["attempts"]==expected_attempts,(name,r)
        assert r["signing_performed"] is False and mod.key_metadata(key)==before
        if stop_code:assert r.get("stop_code")==stop_code,(name,r)
        assert len(list(out.glob("attempt-*-started.json")))==expected_attempts
        assert not any(x.name.startswith("new-crl") for x in out.iterdir())
        results.append(dict(name=name,passed=True,exit=rc,attempts=expected_attempts,signing=False,key_metadata_preserved=True))
    prompt=b"Enter pass phrase for";nextword=b"Type NEXT here"
    terminal_case("correct-hidden-input",[(prompt,FIXTURE_PASSWORD)],True,1)
    terminal_case("wrong-then-owner-stop",[(prompt,b"wrong-synthetic"),(nextword,b"STOP")],False,1)
    terminal_case("wrong-next-correct",[(prompt,b"wrong-synthetic"),(nextword,b"NEXT"),(prompt,FIXTURE_PASSWORD)],True,2)
    terminal_case("three-attempt-limit",[(prompt,b"wrong-synthetic"),(nextword,b"NEXT"),(prompt,b"wrong-synthetic"),(nextword,b"NEXT"),(prompt,b"wrong-synthetic")],False,3)
    terminal_case("wrong-public-identity",[(prompt,FIXTURE_PASSWORD)],False,1,"VALID_KEY_WRONG_PUBLIC_IDENTITY",wrong_identity=True)
    terminal_case("changed-preservation-pin",[],False,0,"PRESERVATION_PIN_CHANGED",mutate=True)
    terminal_case("expired-window",[],False,0,"CHECK_WINDOW",expired=True)
    out,ctx,args=context("noninteractive-refused")
    r=subprocess.run(args,stdin=subprocess.DEVNULL,capture_output=True,timeout=10)
    record=json.loads((out/"result.json").read_text())
    assert r.returncode==1 and record["stop_code"]=="OWNER_LOCAL_TTY_REQUIRED" and record["attempts"]==0
    results.append(dict(name="noninteractive-refused",passed=True,attempts=0,signing=False))
    cmd=mod.command(ctx,out/"test-public.der")
    assert cmd[1]=="pkey" and "-pubout" in cmd and "-check" in cmd and "-text" not in cmd and "-passin" not in cmd and "-passout" not in cmd
    results.append(dict(name="fixed-public-output-operation",passed=True,signing=False))
    report=dict(synthetic_only=True,owner_key_opened=False,owner_password_used=False,passed=len(results),failed=0,
                helper_sha256=sha(HELPER.read_bytes()),openssl_sha256=sha(OPENSSL.read_bytes()),cases=results)
    (ROOT/"synthetic-results.json").write_text(json.dumps(report,indent=2)+"\n")
    print(json.dumps(report,indent=2))
if __name__=="__main__":main()
