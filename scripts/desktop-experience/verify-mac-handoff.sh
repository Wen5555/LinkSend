#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0
printf 'OWNED_PROCESSES\n'
ps -axo pid=,comm= | awk '/LinkSendE0Share|LinkSendE0Extension/ {print}'
printf 'GROUP_RECEIPTS\n'
python3 - <<'PY'
import json, pathlib, hashlib
root=pathlib.Path('/Users/wen/Library/Group Containers/group.com.linksend.e0.share')
print('group_container_exists='+str(root.exists()))
if root.exists():
    for path in root.glob('*.json'):
        print(path.name+'='+path.read_text())
    for path in root.glob('*/captured.json'):
        receipt=json.loads(path.read_text())
        owned=path.parent/'fixture.payload'
        print(json.dumps({'request':receipt['request'],'extension_pid':receipt['extension_pid'],
          'bytes':receipt['bytes'],'current_hash_matches':owned.exists() and hashlib.sha256(owned.read_bytes()).hexdigest()==receipt['sha256'],
          'source_representation_still_exists':pathlib.Path(receipt['source_representation']).exists()}))
        for name in ['open-result.json','host-verified.json']:
            result=path.parent/name
            if result.exists(): print(name+'='+result.read_text())
PY
printf 'EXTENSION_LOG\n'
log show --last 5m --style compact --predicate 'process == "LinkSendE0Extension"' | tail -35
