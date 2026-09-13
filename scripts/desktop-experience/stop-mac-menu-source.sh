#!/bin/sh
set -eu
python3 - <<'PY'
import os, signal, subprocess
expected='/Users/wen/linksend-desktop-experience/e0/mac-share-prototype/menu-source'
for line in subprocess.check_output(['ps','-axo','pid=,comm='],text=True).splitlines():
    parts=line.strip().split(None,1)
    if len(parts)==2 and parts[1]==expected:
        print('STOP_OWNED_MENU_SOURCE='+parts[0],flush=True)
        os.kill(int(parts[0]),signal.SIGTERM)
PY
