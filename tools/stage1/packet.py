"""Strict read-only approval bridge for Windows PowerShell5.1."""
import datetime as dt,json,pathlib,sys
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
import guard
try:
    raw=sys.stdin.buffer.read(131076)
    if raw.startswith(bytes.fromhex('efbbbf')):raw=raw[3:]
    if len(raw)>131072:raise guard.Stop('packet-too-large')
    q=guard.strict_json(raw.decode('utf-8'))
    if q['mode']=='approval_pair':
        p=guard.validate_approvals(q['review'],q['owner'],q['context'],dt.datetime.now(dt.timezone.utc))
    elif q['mode']=='approval':
        p=guard.validate_packet(q['comment'],q['role'],q['sha'],q['issue'],dt.datetime.now(dt.timezone.utc))
    elif q['mode']=='json':p=guard.strict_json(q['body'])
    else:raise guard.Stop('bridge-mode')
    print(json.dumps(p,ensure_ascii=True));sys.exit(0)
except Exception:
    print('{"decision":"NO-GO","code":"strict-packet-rejected"}');sys.exit(2)
