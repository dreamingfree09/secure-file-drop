import argparse,json,pathlib,sys,unittest
HERE=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
class Results(unittest.TextTestResult):
    subtests=0
    def addSubTest(self,test,subtest,outcome):
        self.subtests+=1
        super().addSubTest(test,subtest,outcome)
p=argparse.ArgumentParser();p.add_argument('--output',type=pathlib.Path);args=p.parse_args()
suite=unittest.defaultTestLoader.discover(str(HERE),pattern='test_*.py')
r=unittest.TextTestRunner(verbosity=2,resultclass=Results).run(suite)
out={'schema':'SFD_STAGE1_SYNTHETIC_TESTS_V1','tests':r.testsRun,'subtests':r.subtests,'failures':len(r.failures),'errors':len(r.errors),'skipped':len(r.skipped),'host_mutation':False}
if args.output:
    with args.output.open('xb') as f:f.write((json.dumps(out,indent=2)+'\n').encode())
print(json.dumps(out));sys.exit(0 if r.wasSuccessful() and not r.skipped else 1)
