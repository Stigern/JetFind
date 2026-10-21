#!/bin/sh
# Builds JetFind.app. macOS blocks LAN connections from apps that don't
# declare why they need the local network, so the key is added after packaging.
set -e
cd "$(dirname "$0")"
export PATH="$(go env GOPATH)/bin:$PATH"
command -v fyne >/dev/null || go install fyne.io/tools/cmd/fyne@latest

rm -rf JetFind.app
fyne package --os darwin --release
plutil -insert NSLocalNetworkUsageDescription \
  -string "JetFind scans your local network for JetKVM devices." \
  JetFind.app/Contents/Info.plist
codesign --force --deep --sign - JetFind.app
echo "Built JetFind.app"
