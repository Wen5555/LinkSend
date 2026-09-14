#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
app="$root/LinkSend E0 Share.app"
ext="$app/Contents/PlugIns/LinkSendE0.appex"
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app"
pluginkit -a "$ext"
pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension
cat > "$root/inspect-native.swift" <<'SWIFT'
import Cocoa
import ApplicationServices
print("ACCESSIBILITY_TRUSTED=\(AXIsProcessTrusted())")
let file = URL(fileURLWithPath: "/Users/wen/linksend-desktop-experience/e0/share-fixture.txt")
try Data("LinkSend E0 native Share Extension fixture\n".utf8).write(to: file, options: .withoutOverwriting)
let services = NSSharingService.sharingServices(forItems: [file])
for service in services { print("SHARE_SERVICE=\(service.title)") }
SWIFT
xcrun swift "$root/inspect-native.swift"
open -n "$app"
