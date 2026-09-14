#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
xcrun swiftc -parse-as-library -swift-version 5 -framework Cocoa "$root/MenuSource.swift" -o "$root/menu-source"
python3 - <<'PY'
import subprocess,sys
try:
    result=subprocess.run(['/Users/wen/linksend-desktop-experience/e0/mac-share-prototype/menu-source','--service-only'],timeout=45)
    sys.exit(result.returncode)
except subprocess.TimeoutExpired:
    print('E0_SOURCE_TIMEOUT',flush=True)
    sys.exit(124)
PY
