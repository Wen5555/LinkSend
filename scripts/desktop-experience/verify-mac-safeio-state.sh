#!/bin/sh
set -eu
pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension
python3 - <<'PY'
import pathlib,subprocess,sys
root='/Users/wen/linksend-desktop-experience/e0/mac-share-prototype/'
owned=[]
for line in subprocess.check_output(['ps','-axo','pid=,comm='],text=True).splitlines():
    fields=line.strip().split(None,1)
    if len(fields)==2 and fields[1].startswith(root): owned.append(fields)
print('OWNED_PROCESS_COUNT='+str(len(owned)))
for item in owned: print(item)
group=pathlib.Path('/Users/wen/Library/Group Containers/group.com.linksend.e0.share')
print('CAPTURED_RECEIPT_COUNT='+str(len(list(group.glob('*/captured.json')))))
if owned: sys.exit(1)
PY
