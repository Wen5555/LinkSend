#!/bin/sh
set -eu
sdk=$(xcrun --show-sdk-path)
sed -n '/standardShareMenuItem/,+12p' "$sdk/System/Library/Frameworks/AppKit.framework/Headers/NSSharingService.h"
log show --last 30m --style compact --predicate 'process == "LinkSendE0Extension" AND (eventMessage CONTAINS "principal" OR eventMessage CONTAINS "class" OR eventMessage CONTAINS "controller")' | tail -30
