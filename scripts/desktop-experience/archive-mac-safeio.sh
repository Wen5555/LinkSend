#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
archive=/tmp/linksend-e0-mac-safeio-review.tar.gz
test ! -e "$archive"
tar -czf "$archive" -C "$root" 'LinkSend E0 Share.app'
shasum -a 256 "$archive"
