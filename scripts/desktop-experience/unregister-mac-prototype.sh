#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
app="$root/LinkSend E0 Share.app"
cat > "$root/stop-host.swift" <<'SWIFT'
import Cocoa
let expected = "/Users/wen/linksend-desktop-experience/e0/mac-share-prototype/LinkSend E0 Share.app"
for app in NSRunningApplication.runningApplications(withBundleIdentifier: "com.linksend.e0.sharehost") {
    guard app.bundleURL?.path == expected else { fatalError("HOST_PATH_MISMATCH") }
    print("HOST_TERMINATE_REQUEST_ACCEPTED=\(app.terminate()) PID=\(app.processIdentifier)")
}
SWIFT
xcrun swift "$root/stop-host.swift"
if pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension | grep -F 'com.linksend.e0.sharehost.extension' >/dev/null; then
    /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -u "$app"
    # lsregister may already have removed the embedded plug-in.
    if pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension | grep -F 'com.linksend.e0.sharehost.extension' >/dev/null; then
        pluginkit -r "$app/Contents/PlugIns/LinkSendE0.appex"
    fi
fi
sleep 2
pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension
ps -axo pid=,comm= | awk '/LinkSendE0Share|LinkSendE0Extension/ {print}'
