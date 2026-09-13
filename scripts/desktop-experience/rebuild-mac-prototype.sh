#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
sh "$root/build.sh"
app="$root/LinkSend E0 Share.app"
ext="$app/Contents/PlugIns/LinkSendE0.appex"
file "$ext/Contents/MacOS/LinkSendE0Extension"
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app"
pluginkit -a "$ext"
sleep 2
pluginkit -m -A -D -v -i com.linksend.e0.sharehost.extension
