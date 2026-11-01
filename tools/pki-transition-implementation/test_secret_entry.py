"""Native masked form/private anonymous-pipe tests. Never opens an Owner prompt."""
import datetime,json,os,pathlib,struct,subprocess,sys,unittest,uuid
HERE=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
from fence import create_workspace
EXE=None
def execute(ctx,mode="--pipe-selftest"):
    import msvcrt
    rd,wr=os.pipe();response_rd,response_wr=os.pipe()
    inherited=[msvcrt.get_osfhandle(rd),msvcrt.get_osfhandle(response_wr)]
    for handle in inherited:os.set_handle_inheritable(handle,True)
    si=subprocess.STARTUPINFO();si.lpAttributeList={"handle_list":inherited}
    try:
        p=subprocess.Popen([str(EXE),mode,*map(str,inherited)],startupinfo=si,close_fds=True,
                           stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    finally:
        for handle in inherited:os.set_handle_inheritable(handle,False)
        os.close(rd);os.close(response_wr)
    data=json.dumps(ctx).encode();os.write(wr,struct.pack("<I",len(data))+data);os.close(wr)
    out,err=p.communicate(timeout=8)
    with os.fdopen(response_rd,"rb") as stream:body=stream.read(4096)
    return p.returncode,out,err,body

class SecretTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.identity=json.loads(subprocess.check_output([str(EXE),"--identity"],text=True))
    def context(self):
        root=create_workspace()
        subprocess.run([str(EXE),"--protect-fixture-root",str(root)],check=True,capture_output=True)
        return {"schema":"SFD_SYNTHETIC_SECRET_INPUT_V1","fixture_root":str(root),"lease_id":str(uuid.uuid4()),"fixture_only":True,"real_actions":False,
          "scope":"create_issuer","owner_sid_sha256":self.identity["sid_sha256"],"parent_pid":os.getpid(),
          "expires":(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(seconds=60)).isoformat(),
          "public_review_text":"SYNTHETIC create_issuer No real action; root identities UNKNOWN"}
    def test_native_fields_and_nonpreselected_phase_consent(self):
        r=json.loads(subprocess.check_output([str(EXE),"--self-test"],text=True))
        self.assertTrue(r["masked"]);self.assertTrue(r["confirm_required"]);self.assertFalse(r["real_actions_enabled"])
    def test_secret_transferred_only_on_private_pipe(self):
        code,out,err,body=execute(self.context());self.assertEqual(code,0)
        n=struct.unpack("<I",body[:4])[0];secret=body[4:]
        self.assertEqual(len(secret),n);self.assertTrue(secret.startswith(b"SYNTHETIC-ONLY-"))
        self.assertNotIn(secret,out+err);self.assertEqual(json.loads(out)["real_action"],False)
    def test_expired_lease_rejected(self):
        c=self.context();c["expires"]="2000-01-01T00:00:00Z"
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")
    def test_wrong_user_binding_rejected(self):
        c=self.context();c["owner_sid_sha256"]="0"*64
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")
    def test_wrong_actual_parent_rejected(self):
        c=self.context();c["parent_pid"]=1
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")
    def test_real_action_flag_rejected(self):
        c=self.context();c["real_actions"]=True
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")
    def test_no_blanket_approval_scope(self):
        c=self.context();c["scope"]="approve_all"
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")
    def test_excessive_lease_rejected(self):
        c=self.context();c["expires"]=(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(hours=1)).isoformat()
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")


    def test_input_lease_cannot_be_replayed(self):
        c=self.context();code,_,_,body=execute(c);self.assertEqual(code,0);self.assertTrue(body)
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")
    def test_cancel_consumes_lease_without_secret_or_retry(self):
        c=self.context();code,_,_,body=execute(c,mode="--cancel-selftest")
        self.assertEqual(code,2);self.assertEqual(body,b"")
        code,_,_,body=execute(c);self.assertNotEqual(code,0);self.assertEqual(body,b"")

if __name__=="__main__":
    from context import pins
    execution_source=pins()
    if os.name!="nt" or len(sys.argv)!=2:raise RuntimeError("Windows fixture binary required")
    EXE=pathlib.Path(sys.argv.pop()).resolve()
    result=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(SecretTests))
    receipt={"scope":"synthetic native input controls/private pipe; no Owner interaction","tests_run":result.testsRun,
      "failures":len(result.failures),"errors":len(result.errors),"skips":len(result.skipped),
      "Owner_prompt_opened":False,"real_secret_requested_or_entered":False,"real_cert_store_access":False}
    receipt["executed_source_SHA256"]=execution_source
    receipt["execution_source_unchanged"]=execution_source==pins()
    (HERE/"secret-results.json").write_text(json.dumps(receipt,indent=2)+"\n",newline="\n")
    raise SystemExit(0 if result.wasSuccessful() and execution_source==pins() else 1)
