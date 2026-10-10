#!/bin/sh
# Create a universal signed/notarized installer using existing Developer ID credentials.
set -eu
cd "$(dirname "$0")/../.."
if [ "$#" -ne 4 ]; then
 echo 'usage: build-signed.sh VERSION "Developer ID Application: …" "Developer ID Installer: …" NOTARY_KEYCHAIN_PROFILE' >&2
 exit 2
fi
signed_version="$1"
application_identity="$2"
installer_identity="$3"
notary_profile="$4"
python3 - "$signed_version" <<'PY'
import re,sys
if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', sys.argv[1]):
 raise SystemExit('Invalid version')
PY
[ "$(uname -s)" = Darwin ] || { echo 'Signing requires macOS.' >&2; exit 1; }
case "$application_identity" in 'Developer ID Application: '*) ;; *) echo 'Developer ID Application identity required.' >&2; exit 1;; esac
case "$installer_identity" in 'Developer ID Installer: '*) ;; *) echo 'Developer ID Installer identity required.' >&2; exit 1;; esac
security find-identity -v -p codesigning | grep -F -- "\"$application_identity\"" >/dev/null || { echo 'Application identity unavailable.' >&2; exit 1; }
security find-identity -v | grep -F -- "\"$installer_identity\"" >/dev/null || { echo 'Installer identity unavailable.' >&2; exit 1; }
for signed_tool in codesign pkgbuild productbuild lipo xcrun; do command -v "$signed_tool" >/dev/null; done
# Authenticate before creating build/output files. The profile already exists;
# this script never accepts or stores an Apple password/API key.
xcrun notarytool history --keychain-profile "$notary_profile" --output-format json >/dev/null
signed_dir="$PWD/dist/signed/$signed_version"
[ ! -e "$signed_dir" ] || { echo 'Signed version already exists; inspect before replacing.' >&2; exit 1; }
build_go="${HARBOR_GO:-go}"
export GOCACHE="$PWD/.tools/gocache" GOPATH="$PWD/.tools/gopath"
(cd web && npm ci --no-audit --no-fund && npm run build -- --outDir ../internal/ui/dist --emptyOutDir)
mkdir -p dist/signed
stage="$(mktemp -d "$PWD/dist/signed/.stage-XXXXXX")"
trap 'rm -rf -- "$stage"' EXIT HUP INT TERM
mkdir -p "$stage/payload/bin"
for signed_arch in amd64 arm64; do
 CGO_ENABLED=0 GOOS=darwin GOARCH="$signed_arch" "$build_go" build -trimpath -ldflags "-X main.version=$signed_version" -o "$stage/harbor-$signed_arch" ./cmd/harbor
done
lipo -create "$stage/harbor-amd64" "$stage/harbor-arm64" -output "$stage/payload/bin/harbor"
chmod 755 "$stage/payload/bin/harbor"
codesign --force --options runtime --timestamp --sign "$application_identity" "$stage/payload/bin/harbor"
codesign --verify --strict "$stage/payload/bin/harbor"
python3 scripts/collect-notices.py "$build_go"
cp README.md LICENSE dist/THIRD_PARTY_NOTICES.txt "$stage/payload/"
cp -R docs "$stage/payload/docs"
cp packaging/macos/install.sh packaging/macos/uninstall.sh "$stage/payload/"
pkgbuild --root "$stage/payload" --identifier org.harbor.runner --version "$(printf '%s' "$signed_version" | cut -d- -f1)" --install-location /usr/local/lib/harbor "$stage/component.pkg"
productbuild --package "$stage/component.pkg" --sign "$installer_identity" "$stage/harbor-$signed_version.pkg"
xcrun notarytool submit "$stage/harbor-$signed_version.pkg" --keychain-profile "$notary_profile" --wait --output-format json > "$stage/notarization.json"
python3 - "$stage/notarization.json" <<'PY'
import json,sys
if json.load(open(sys.argv[1])).get('status')!='Accepted': raise SystemExit('Notarization not accepted; do not distribute')
PY
xcrun stapler staple "$stage/harbor-$signed_version.pkg"
xcrun stapler validate "$stage/harbor-$signed_version.pkg"
spctl --assess --type install "$stage/harbor-$signed_version.pkg"
mkdir "$signed_dir"
cp "$stage/harbor-$signed_version.pkg" "$stage/notarization.json" "$signed_dir/"
(cd "$signed_dir" && shasum -a 256 "harbor-$signed_version.pkg" > SHA256SUMS)
echo "Verified signed installer: $signed_dir/harbor-$signed_version.pkg"
