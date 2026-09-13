#!/bin/sh
set -eu
printf 'OS\n'; sw_vers; uname -m
printf 'INTERFACES\n'; ifconfig
printf 'ROUTES\n'; netstat -rn -f inet; netstat -rn -f inet6
printf 'LISTENERS\n'; lsof -nP -iTCP -sTCP:LISTEN || true
printf 'TOOLCHAIN\n'; xcode-select -p; xcrun --find swift; xcrun swift --version; xcodebuild -version || true
printf 'SIGNING_IDENTITIES\n'; security find-identity -v -p codesigning
printf 'LINKSEND_APP\n'; if test -d /Applications/LinkSend.app; then /usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' /Applications/LinkSend.app/Contents/Info.plist; shasum -a 256 /Applications/LinkSend.app/Contents/MacOS/*; codesign -dv /Applications/LinkSend.app 2>&1; fi
printf 'BUILD_TOOLS\n'; find /Users/wen/linksend-desktop-six -maxdepth 3 -name use-toolchain.sh -o -name go -o -name wails3 2>/dev/null || true
