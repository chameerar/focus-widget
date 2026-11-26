#!/usr/bin/env bash
# Builds Focus Widget.app for one architecture and wraps it in a DMG in dist/.
# Usage, from the repository root on macOS: build/macos/package.sh arm64|amd64
set -euo pipefail

arch=${1:?usage: build/macos/package.sh arm64|amd64}
case $arch in
  arm64) label=arm64 ;;
  amd64) label=x64 ;;
  *) echo "unknown architecture: $arch" >&2; exit 1 ;;
esac
version=$(tr -d '[:space:]' < VERSION)
name="Focus Widget"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
stage="$work/dmg"
app="$stage/$name.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" dist

# private_mac_apis lets the webview draw without a background, which the transparent window needs.
CGO_ENABLED=1 GOOS=darwin GOARCH=$arch MACOSX_DEPLOYMENT_TARGET=12.0 \
  go build -tags production,private_mac_apis -trimpath -ldflags "-s -w" \
  -o "$app/Contents/MacOS/focus-widget" .

sed "s/@VERSION@/$version/g" build/macos/Info.plist > "$app/Contents/Info.plist"

iconset="$work/icon.iconset"
mkdir "$iconset"
for s in 16 32 128 256 512; do
  sips -z "$s" "$s" assets/icon.png --out "$iconset/icon_${s}x${s}.png" > /dev/null
  [ "$s" -lt 512 ] && sips -z $((s * 2)) $((s * 2)) assets/icon.png --out "$iconset/icon_${s}x${s}@2x.png" > /dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/icon.icns"

# Ad-hoc signature only: there is no Developer ID, so the app is not notarized.
codesign --force --sign - "$app"

ln -s /Applications "$stage/Applications"
dmg="dist/focus-widget-$version-mac-$label.dmg"
hdiutil create -volname "$name" -srcfolder "$stage" -fs HFS+ -format UDZO -ov "$dmg" > /dev/null
echo "$dmg"
