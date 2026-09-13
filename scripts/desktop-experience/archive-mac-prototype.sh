#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0/mac-share-prototype
tar -czf /tmp/linksend-e0-mac-share-review.tar.gz -C "$root" 'LinkSend E0 Share.app'
shasum -a 256 /tmp/linksend-e0-mac-share-review.tar.gz
