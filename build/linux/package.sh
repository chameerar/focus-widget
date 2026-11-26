#!/usr/bin/env bash
# Builds focus-widget for Linux and packages it as a .deb and a .tar.gz in dist/.
# Usage, from the repository root: build/linux/package.sh
# Needs Go, gcc and the GTK 3 and WebKitGTK 4.1 development files
# (Debian and Ubuntu: libgtk-3-dev libwebkit2gtk-4.1-dev).
set -euo pipefail

version=$(tr -d '[:space:]' < VERSION)
arch=$(go env GOARCH) # amd64 or arm64, which are also the Debian names
case $arch in
  amd64) label=x64 ;;
  *) label=$arch ;;
esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p dist

# gtk3 selects GTK 3 and WebKitGTK 4.1, which Ubuntu 22.04 and Debian 12 ship, over GTK 4.
CGO_ENABLED=1 go build -tags production,gtk3 -trimpath -ldflags "-s -w" -o "$work/focus-widget" .

# The system's WebKitGTK and GTK are used, so the package only declares them.
root="$work/deb"
install -D -m 755 "$work/focus-widget" "$root/usr/bin/focus-widget"
install -D -m 644 build/linux/focus-widget.desktop "$root/usr/share/applications/focus-widget.desktop"
install -D -m 644 assets/icon.png "$root/usr/share/icons/hicolor/512x512/apps/focus-widget.png"
install -D -m 644 LICENSE "$root/usr/share/doc/focus-widget/copyright"
mkdir -p "$root/DEBIAN"
cat > "$root/DEBIAN/control" << EOF
Package: focus-widget
Version: $version
Architecture: $arch
Maintainer: Focus Widget contributors
Installed-Size: $(du -sk "$root/usr" | cut -f1)
Depends: libgtk-3-0t64 | libgtk-3-0, libwebkit2gtk-4.1-0
Section: utils
Priority: optional
Homepage: https://github.com/chameerar/focus-widget
Description: Desktop countdown widget with a segmented dial and quick timers
 A small always-on-top timer with quick presets, a tray countdown and an alarm chime.
EOF
dpkg-deb --root-owner-group --build "$root" "dist/focus-widget-$version-linux-$label.deb" > /dev/null

# The same files for other distributions: needs GTK 3 and WebKitGTK 4.1 installed.
tarroot="$work/focus-widget-$version"
mkdir -p "$tarroot"
cp "$work/focus-widget" build/linux/focus-widget.desktop LICENSE "$tarroot/"
cp assets/icon.png "$tarroot/focus-widget.png"
tar -C "$work" -czf "dist/focus-widget-$version-linux-$label.tar.gz" "focus-widget-$version"

echo "dist/focus-widget-$version-linux-$label.deb"
echo "dist/focus-widget-$version-linux-$label.tar.gz"
