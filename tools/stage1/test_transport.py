import json,pathlib,subprocess,sys,unittest
HERE=pathlib.Path(__file__).resolve().parent
class TransportTests(unittest.TestCase):
 def invoke(self,raw):
  return subprocess.run([sys.executable,'-I','-B',str(HERE/'packet.py')],input=raw,capture_output=True)
 def test_native_utf8_transport(self):
  raw=json.dumps({'mode':'json','body':'{"fixture":true}'}).encode()
  for prefix in [b'',bytes.fromhex('efbbbf')]:
   with self.subTest(prefix=prefix.hex()):
    p=self.invoke(prefix+raw);self.assertEqual(p.returncode,0);self.assertEqual(json.loads(p.stdout),{'fixture':True})
 def test_malformed_or_duplicate_transport_denied(self):
  cases=[bytes.fromhex('efbbbfefbbbf')+b'{}',b'{"mode":"json","mode":"json","body":"{}"}',json.dumps({'mode':'json','body':'{"fixture":true,"fixture":false}'}).encode(),b'{"mode":"json","body":NaN}',json.dumps({'mode':'json','body':'x'*131073}).encode()]
  for raw in cases:
   with self.subTest(raw=raw[:80]):
    p=self.invoke(raw);self.assertNotEqual(p.returncode,0)
if __name__=='__main__':unittest.main()
