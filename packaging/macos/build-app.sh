#!/bin/sh
# Build the universal desktop app and a drag-to-Applications disk image.
set -eu
cd "$(dirname "$0")/../.."
app_version="${1:-0.1.0-dev}"
python3 - "$app_version" <<'PY'
import re,sys
if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', sys.argv[1]):
 raise SystemExit('Invalid version')
PY
[ "$(uname -s)" = Darwin ] || { echo 'Mac app builds require macOS and Xcode command-line tools.' >&2; exit 1; }
app_output="${2:-$PWD/dist/desktop/$app_version}"
case "$app_output" in /*) ;; *) app_output="$PWD/$app_output";; esac
[ ! -e "$app_output" ] || { echo 'App output already exists; choose a new version/directory.' >&2; exit 1; }
build_go="${HARBOR_GO:-go}"
export GOCACHE="$PWD/.tools/gocache" GOPATH="$PWD/.tools/gopath"
if [ ! -d web/node_modules ]; then (cd web && npm ci --no-audit --no-fund); fi
(cd web && npm run build -- --outDir ../internal/ui/dist --emptyOutDir)
"$build_go" mod download all
python3 scripts/collect-notices.py "$build_go"
mkdir -p "$(dirname "$app_output")" "$PWD/.tools/swift-module-cache"
stage="$(mktemp -d "$(dirname "$app_output")/.app-stage-XXXXXX")"
trap 'rm -rf -- "$stage"' EXIT HUP INT TERM
app="$stage/image/Harbor.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
for app_arch in arm64 amd64; do
 CGO_ENABLED=0 GOOS=darwin GOARCH="$app_arch" "$build_go" build -trimpath -buildvcs=false -ldflags "-X main.version=$app_version" -o "$stage/runner-$app_arch" ./cmd/harbor
 case "$app_arch" in arm64) swift_arch=arm64;; amd64) swift_arch=x86_64;; esac
 xcrun swiftc -parse-as-library -O -module-cache-path "$PWD/.tools/swift-module-cache" -target "$swift_arch-apple-macosx13.0" packaging/macos/Harbor.swift -o "$stage/app-$app_arch" -framework AppKit -framework WebKit
done
lipo -create "$stage/runner-arm64" "$stage/runner-amd64" -output "$app/Contents/Resources/harbor"
lipo -create "$stage/app-arm64" "$stage/app-amd64" -output "$app/Contents/MacOS/Harbor"
python3 - "$app_version" packaging/macos/Info.plist "$app/Contents/Info.plist" <<'PY'
import pathlib,sys
pathlib.Path(sys.argv[3]).write_text(pathlib.Path(sys.argv[2]).read_text().replace('HARBOR_NUMERIC_VERSION',sys.argv[1].split('-')[0]).replace('HARBOR_VERSION',sys.argv[1]))
PY
cp packaging/macos/Harbor.icns LICENSE dist/THIRD_PARTY_NOTICES.txt "$app/Contents/Resources/"
# Apple Silicon executables need local ad-hoc signatures; no certificate is used.
codesign --force --sign - "$app/Contents/Resources/harbor"
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"
ln -s /Applications "$stage/image/Applications"
cat > "$stage/image/Install Harbor.txt" <<'TEXT'
INSTALL HARBOR

1. Drag Harbor into Applications.
2. Open Harbor from Applications.

Harbor is distributed without Developer ID signing or Apple notarization.
If macOS blocks the first launch, open System Settings > Privacy & Security,
click Open Anyway for Harbor, then confirm Open.

Alternatively, after copying Harbor to Applications, open Terminal and run:

    xattr -dr com.apple.quarantine "/Applications/Harbor.app"

Then open Harbor again. This removes the download quarantine attribute only
from Harbor and its bundled files. Repeat the approval for a newer download
if macOS requests it.

Harbor sets up its background runner and opens your ships.
Closing the window or quitting the interface leaves ships running.
Use the Harbor menu to stop or uninstall it. Ship data is retained.
This is an experimental alpha. Use disposable ships.
TEXT
hdiutil create -quiet -fs HFS+ -volname Harbor -srcfolder "$stage/image" -format UDZO "$stage/Harbor-$app_version-macos-universal.dmg"
mkdir "$app_output"
mv "$app" "$stage/Harbor-$app_version-macos-universal.dmg" "$app_output/"
(cd "$app_output" && shasum -a 256 "Harbor-$app_version-macos-universal.dmg" > SHA256SUMS)
echo "Mac app and disk image: $app_output"
echo 'Distributed without Developer ID signing or notarization. First-launch instructions are included in the disk image.'
