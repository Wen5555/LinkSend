#!/bin/sh
set -eu
cd /Users/wen/linksend-desktop-experience/e0/mac-share-prototype
xcrun swiftc -swift-version 5 FixtureIO.swift FixtureIOTests.swift -o fixture-io-tests
./fixture-io-tests
sh build.sh
