#!/usr/bin/env bash
# Builds focus-widget.exe for x64 and its NSIS installer in dist/.
# Usage, from the repository root (Git Bash on Windows, or any OS with makensis): build/windows/package.sh
set -euo pipefail

version=$(tr -d '[:space:]' < VERSION)
work=$(mktemp -d)
trap 'rm -rf "$work" rsrc_windows_amd64.syso' EXIT
mkdir -p dist

# Icon, version details and a manifest (DPI awareness, modern controls), linked in by go build.
go tool go-winres simply --arch amd64 --icon assets/icon.png --manifest gui \
  --product-name "Focus Widget" --file-description "Focus Widget" --original-filename focus-widget.exe \
  --product-version "$version" --file-version "$version" \
  --copyright "Copyright (c) 2026 Focus Widget contributors"

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -tags production -trimpath -ldflags "-s -w -H windowsgui" -o "$work/focus-widget.exe" .

# makensis is a Windows program there, so it needs Windows paths, and it may not be on PATH.
native() { if command -v cygpath > /dev/null; then cygpath -w "$1"; else echo "$1"; fi; }
nsis=makensis
if ! command -v makensis > /dev/null && [ -x "/c/Program Files (x86)/NSIS/makensis.exe" ]; then
  nsis="/c/Program Files (x86)/NSIS/makensis.exe"
fi
out="dist/focus-widget-$version-win-x64.exe"
"$nsis" -V2 -DVERSION="$version" -DSRC="$(native "$work")" -DOUTFILE="$(native "$PWD/$out")" \
  "$(native build/windows/installer.nsi)"
echo "$out"
