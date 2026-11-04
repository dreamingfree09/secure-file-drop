"""Require both reviewed parser failures and current success without editing source."""
import hashlib,json,pathlib,subprocess,tempfile,urllib.request
repo=pathlib.Path(__file__).resolve().parents[2]
baselines=[
 ("139490c324b1217044278e66df870c3570f9cbd2","3666190064fda322f80e269cd0db87233f73975f9f2d6b4477181d00b5096896",["case denial_leading_text acceptance mismatch"]),
 ("99d6a09688b7bc2b41aee1ad8629b4dbdfe21fa2","ee071ff7c337cd8ffcfcaabc8006709f9ada52bf90d555ce98dd41e6faa7d0e0",["case denial_adjacent_encoding acceptance mismatch","case denial_adjacent_standalone acceptance mismatch"]),
 ("cfc1ad6af9308ed7faa7da64443490dd36bfa9ab","73696a6462a78a1a4406cefe519515073f97d30a3c0a1e42af27ea2e5ac011e0",["case denial_adjacent_root_attributes acceptance mismatch","case denial_adjacent_code_attributes acceptance mismatch"]),
]
args=["go","test","./internal/server","-run","^TestRailwayPrivacyAttestation$","-count=1","-v"]
with tempfile.TemporaryDirectory(prefix="sfd-xml-denial-regression-") as temporary:
 root=pathlib.Path(temporary)
 for revision,expected,markers in baselines:
  url="https://raw.githubusercontent.com/dreamingfree09/secure-file-drop/"+revision+"/internal/server/railway_privacy.go"
  with urllib.request.urlopen(url,timeout=20) as response:old=response.read(262145)
  assert len(old)<=262144 and hashlib.sha256(old).hexdigest()==expected,"pinned source integrity failure"
  source=root/(revision+".go");source.write_bytes(old)
  overlay=root/(revision+".json");overlay.write_text(json.dumps({"Replace":{str(repo/"internal/server/railway_privacy.go"):str(source)}}),encoding="utf-8")
  before=subprocess.run(args[:2]+["-overlay",str(overlay)]+args[2:],cwd=repo,capture_output=True,text=True,timeout=90)
  assert before.returncode!=0 and all(marker in before.stdout for marker in markers),"old parser did not exhibit the exact reviewed failure"
 after=subprocess.run(args,cwd=repo,capture_output=True,text=True,timeout=90)
 assert after.returncode==0 and "SDK attestation cases executed: 73" in after.stdout and "--- PASS: TestRailwayPrivacyAttestation" in after.stdout,"corrected parser did not pass all real SDK cases"
 assert "--- SKIP:" not in after.stdout
 print(json.dumps({"reviewed_revision":baselines[0][0],"reviewed_source_SHA256":baselines[0][1],"reviewed_leading_text_regression_failed":True,"separator_reviewed_revision":baselines[1][0],"separator_source_SHA256":baselines[1][1],"reviewed_attribute_separators_regression_failed":True,"elements_reviewed_revision":baselines[2][0],"elements_source_SHA256":baselines[2][1],"reviewed_element_attributes_regression_failed":True,"corrected_SDK_cases":73,"corrected_parser_passed":True,"source_overwritten":False}))
