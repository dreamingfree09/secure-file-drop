#!/usr/bin/env python3
"""Go E2E adapter for the same ownership-checked real-stack acceptance harness."""
import json, os, pathlib, sys
import acceptance

def main():
    layout=os.environ['SFD_E2E_HASH_LAYOUT']
    assert layout in ['bigint','bytea']
    root=pathlib.Path(os.environ['SFD_E2E_OUTPUT_ROOT'])/('sfd-auth-fixture'+('-bytea' if layout=='bytea' else ''))
    assert root.is_absolute() and not root.exists()
    sys.argv=[str(pathlib.Path(acceptance.__file__)), '--backend-image',os.environ['SFD_E2E_BACKEND_IMAGE'],'--postgres-image',os.environ['SFD_E2E_POSTGRES_IMAGE'],'--storage-image',os.environ['SFD_E2E_STORAGE_IMAGE'],'--output',str(root),'--hash-layout',layout]
    if os.environ.get('SFD_E2E_ROLLBACK_IMAGE'):sys.argv+=['--rollback-image',os.environ['SFD_E2E_ROLLBACK_IMAGE']]
    if os.environ.get('SFD_E2E_BROWSER_MODULE'):sys.argv+=['--browser-module',os.environ['SFD_E2E_BROWSER_MODULE']]
    acceptance.main()
    result=json.loads((root/'result.json').read_text())
    assert not result['failures'] and result['existing_services_unchanged'] and result['owned_cleanup_complete'] and len(result['checks'])>=27

if __name__=='__main__':main()
