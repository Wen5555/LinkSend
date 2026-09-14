#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
app="$root/LinkSend E0 Share.app"
ext="$app/Contents/PlugIns/LinkSendE0.appex"
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app"
pluginkit -a "$ext"
pluginkit -e use -i com.linksend.e0.sharehost.extension
pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension
xcrun swiftc -parse-as-library -swift-version 5 -framework Cocoa "$root/MenuSource.swift" -o "$root/menu-source"
"$root/menu-source" --menu-only
"$root/menu-source" --service-only
log show --last 2m --style compact --predicate 'process == "LinkSendE0Extension" AND eventMessage CONTAINS "E0_"'
