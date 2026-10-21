#!/bin/sh
# Builds JetFind.app. macOS blocks LAN connections from apps that don't
# declare why they need the local network, so the key is added after packaging.
set -e
cd "$(dirname "$0")"
export PATH="$(go env GOPATH)/bin:$PATH"
command -v fyne >/dev/null || go install fyne.io/tools/cmd/fyne@latest

rm -rf JetFind.app
fyne package --os darwin --release

# Add the other architecture so the app runs on both Apple Silicon and Intel.
other=amd64
[ "$(go env GOARCH)" = amd64 ] && other=arm64
tmp=$(mktemp)
CGO_ENABLED=1 GOARCH=$other go build -ldflags "-s -w" -o "$tmp" . 2>&1 | grep -v "duplicate libraries" || true
lipo -create JetFind.app/Contents/MacOS/jetfind "$tmp" -output JetFind.app/Contents/MacOS/jetfind
rm -f "$tmp"

plutil -insert NSLocalNetworkUsageDescription \
  -string "JetFind scans your local network for JetKVM devices." \
  JetFind.app/Contents/Info.plist
codesign --force --deep --sign - JetFind.app
echo "Built JetFind.app"
