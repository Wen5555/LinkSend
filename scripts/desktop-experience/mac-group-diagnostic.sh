#!/bin/sh
set -eu
log show --last 5m --style compact --predicate 'process == "LinkSendE0Extension" AND eventMessage CONTAINS "E0_"'
python3 - <<'PY'
import pathlib
root=pathlib.Path('/Users/wen/Library/Group Containers/group.com.linksend.e0.share')
for p in root.glob('*.json'): print(p.name+'='+p.read_text())
for p in root.glob('*/captured.json'): print(p.name+'='+p.read_text())
PY
