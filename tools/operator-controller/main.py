"""Codex-operated entry point. No test runtime/class injection or unsafe mode."""
import argparse,json
from policy import Controller,GateFailure
from runtime import Runtime
def main():
    p=argparse.ArgumentParser();p.add_argument('--context',required=True);p.add_argument('--verify-only',action='store_true');a=p.parse_args()
    try:
        r=Runtime(a.context)
        if a.verify_only:r.authority();print(json.dumps({'authority_verified':True,'live_execution':False}));return 0
        result=Controller(r).execute()
        print(json.dumps(result));return 0
    except Exception:
        print('CONTROLLER STOPPED — fixed-stage private receipt only; no credentials disclosed.');return 1
if __name__=='__main__':raise SystemExit(main())
