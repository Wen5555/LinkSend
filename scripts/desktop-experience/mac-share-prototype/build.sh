#!/bin/sh
set -eu
cd /Users/wen/linksend-desktop-experience/e0/mac-share-prototype
app="$PWD/LinkSend E0 Share.app"
ext="$app/Contents/PlugIns/LinkSendE0.appex"
mkdir -p "$app/Contents/MacOS" "$ext/Contents/MacOS"
cp Host-Info.plist "$app/Contents/Info.plist"
cp Extension-Info.plist "$ext/Contents/Info.plist"
xcrun swiftc -swift-version 5 -target arm64-apple-macos13.0 -framework Cocoa -framework CryptoKit FixtureIO.swift Host.swift -o "$app/Contents/MacOS/LinkSendE0Share"
xcrun swiftc -swift-version 5 -target arm64-apple-macos13.0 -framework Cocoa -framework UniformTypeIdentifiers -framework CryptoKit -application-extension -parse-as-library -Xlinker -e -Xlinker _NSExtensionMain FixtureIO.swift ShareViewController.swift -o "$ext/Contents/MacOS/LinkSendE0Extension"
codesign --force --sign - --entitlements entitlements.plist "$ext"
codesign --force --sign - --entitlements entitlements.plist "$app"
codesign --verify --deep --strict --verbose=2 "$app"
shasum -a 256 "$app/Contents/MacOS/LinkSendE0Share" "$ext/Contents/MacOS/LinkSendE0Extension"
codesign -d --entitlements :- "$ext"
