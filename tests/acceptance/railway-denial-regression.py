"""Require real old-parser failure and current-parser success without editing source."""
import hashlib,json,pathlib,subprocess,tempfile,urllib.request
repo=pathlib.Path(__file__).resolve().parents[2]
revision="139490c324b1217044278e66df870c3570f9cbd2"
expected="3666190064fda322f80e269cd0db87233f73975f9f2d6b4477181d00b5096896"
url="https://raw.githubusercontent.com/dreamingfree09/secure-file-drop/"+revision+"/internal/server/railway_privacy.go"
with urllib.request.urlopen(url,timeout=20) as response:old=response.read(262145)
assert len(old)<=262144 and hashlib.sha256(old).hexdigest()==expected,"pinned source integrity failure"
with tempfile.TemporaryDirectory(prefix="sfd-xml-denial-regression-") as temporary:
 root=pathlib.Path(temporary);source=root/"received.go";source.write_bytes(old)
 overlay=root/"overlay.json";overlay.write_text(json.dumps({"Replace":{str(repo/"internal/server/railway_privacy.go"):str(source)}}),encoding="utf-8")
 args=["go","test","./internal/server","-run","^TestRailwayPrivacyAttestation$","-count=1","-v"]
 before=subprocess.run(args[:2]+["-overlay",str(overlay)]+args[2:],cwd=repo,capture_output=True,text=True,timeout=90)
 assert before.returncode!=0 and "case denial_leading_text acceptance mismatch" in before.stdout,"old parser did not exhibit the exact reviewed failure"
 after=subprocess.run(args,cwd=repo,capture_output=True,text=True,timeout=90)
 assert after.returncode==0 and "SDK attestation cases executed: 62" in after.stdout and "--- PASS: TestRailwayPrivacyAttestation" in after.stdout,"corrected parser did not pass all real SDK cases"
 assert "--- SKIP:" not in after.stdout
 print(json.dumps({"reviewed_revision":revision,"reviewed_source_SHA256":expected,"reviewed_leading_text_regression_failed":True,"corrected_SDK_cases":62,"corrected_parser_passed":True,"source_overwritten":False}))
