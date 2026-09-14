#!/bin/sh
set -eu
root=/Users/wen/linksend-desktop-experience/e0
mkdir -p "$root"
test ! -e "$root/mac-share-prototype"
tar -xzf /tmp/linksend-e0-mac-share-prototype-v1.tar.gz -C "$root"
sh "$root/mac-share-prototype/build.sh"
