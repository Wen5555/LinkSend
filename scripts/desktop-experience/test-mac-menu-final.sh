#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
cd "$root"
sh build.sh
nm -m 'LinkSend E0 Share.app/Contents/PlugIns/LinkSendE0.appex/Contents/MacOS/LinkSendE0Extension' | grep 'OBJC_CLASS.*LinkSendE0ShareViewController'
sh test-native-menu.sh
