#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
sh "$root/build.sh"
app="$root/LinkSend E0 Share.app"
ext="$app/Contents/PlugIns/LinkSendE0.appex"
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app"
pluginkit -a "$ext"
pluginkit -e use -i com.linksend.e0.sharehost.extension
cat > "$root/activate.swift" <<'SWIFT'
import Cocoa
let app = NSApplication.shared
let file = URL(fileURLWithPath: "/Users/wen/linksend-desktop-experience/e0/share-fixture.txt")
let services = NSSharingService.sharingServices(forItems: [file])
for service in services { print("SHARE_SERVICE=\(service.title)") }
guard let service = services.first(where: { $0.title == "LinkSend E0" }) else {
    print("E0_SHARE_SERVICE_NOT_AVAILABLE"); exit(2)
}
print("PERFORM_SYSTEM_SHARE")
service.perform(withItems: [file])
RunLoop.current.run(until: Date(timeIntervalSinceNow: 15))
SWIFT
xcrun swift "$root/activate.swift"
pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension
