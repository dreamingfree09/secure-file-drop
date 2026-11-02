"""Aggregate real evidence; no fabricated successes and no credential output."""
import argparse,json,pathlib,sys,re
ROOT=pathlib.Path(__file__).resolve().parent

def aggregate(receipts):
    ledger=json.loads((ROOT/'acceptance.json').read_text())
    known={c['id']:c for c in ledger['checks']}
    seen=set()
    for receipt in receipts:
        if receipt.get('schema')!='SFD_STAGE1_CHECK_RECEIPT_V1':raise ValueError('receipt-schema')
        key=receipt['id']
        if key not in known or key in seen:raise ValueError('receipt-id')
        seen.add(key)
        if receipt['status'] not in ['PASS','FAIL','NOT RUN','BLOCKED']:raise ValueError('receipt-status')
        if receipt['status']=='PASS' and (not isinstance(receipt.get('evidence_sha256'),str) or not re.fullmatch('[a-f0-9]{64}',receipt['evidence_sha256'])):raise ValueError('unbound-pass')
        if known[key]['kind']=='Owner-visual' and receipt.get('operator')!='Owner':raise ValueError('visual-not-automated')
        known[key]['status']=receipt['status'];known[key]['evidence_sha256']=receipt.get('evidence_sha256')
    ledger['authorises_deployment']=False
    ledger['requires_independent_verification']=True
    ledger['accepted']=all(c['status']=='PASS' for c in known.values())
    return ledger
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('receipts',type=pathlib.Path);a=p.parse_args()
    try:r=aggregate(json.loads(a.receipts.read_text()));print(json.dumps(r));sys.exit(0 if r['accepted'] else 2)
    except (ValueError,KeyError,OSError):print('{"accepted":false,"code":"invalid-receipt"}');sys.exit(2)
