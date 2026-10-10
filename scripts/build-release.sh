#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
release_version="${1:-0.1.0-alpha.1}"
python3 - "$release_version" <<'PY'
import re,sys
if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', sys.argv[1]):
    raise SystemExit('Use a version such as 0.1.0-alpha.1')
PY
release_dir="$PWD/dist/releases/$release_version"
[ ! -e "$release_dir" ] || { echo 'Release directory already exists; inspect before replacing.' >&2; exit 1; }
build_go="${HARBOR_GO:-go}"
export GOCACHE="$PWD/.tools/gocache" GOPATH="$PWD/.tools/gopath"
(cd web && npm ci --no-audit --no-fund && npm run build -- --outDir ../internal/ui/dist --emptyOutDir)
"$build_go" mod download all
mkdir -p dist/releases
python3 scripts/collect-notices.py "$build_go"
stage="$(mktemp -d "$PWD/dist/releases/.stage-XXXXXX")"
# Cleanup only the staging directory created by this invocation.
trap 'rm -rf -- "$stage"' EXIT HUP INT TERM
for release_os in darwin linux; do
 for release_arch in amd64 arm64; do
  artifact="harbor-$release_version-$release_os-$release_arch"
  mkdir "$stage/$artifact"
  CGO_ENABLED=0 GOOS="$release_os" GOARCH="$release_arch" "$build_go" build -trimpath -ldflags "-X main.version=$release_version" -o "$stage/$artifact/harbor" ./cmd/harbor
  cp README.md LICENSE CONTRIBUTING.md SECURITY.md CHANGELOG.md dist/THIRD_PARTY_NOTICES.txt "$stage/$artifact/"
  cp -R docs "$stage/$artifact/docs"
  case "$release_os" in darwin) cp packaging/macos/install.sh packaging/macos/uninstall.sh "$stage/$artifact/";; linux) cp packaging/linux/* "$stage/$artifact/";; esac
  python3 - "$stage/$artifact" "$stage/$artifact.tar.gz" <<'PY'
import gzip,pathlib,sys,tarfile
root=pathlib.Path(sys.argv[1])
with open(sys.argv[2],'xb') as target:
 with gzip.GzipFile(filename='',fileobj=target,mode='wb',mtime=0) as compressed:
  with tarfile.open(fileobj=compressed,mode='w') as archive:
   for source in sorted(root.rglob('*')):
    if source.is_dir(): continue
    if not source.is_file() or source.is_symlink(): raise SystemExit('Unexpected package entry')
    info=tarfile.TarInfo('./'+source.relative_to(root).as_posix())
    info.size=source.stat().st_size
    info.mode=0o755 if source.name=='harbor' or source.suffix=='.sh' else 0o644
    with source.open('rb') as content: archive.addfile(info,content)
PY
 done
done
python3 scripts/build-source.py "$release_version" "$stage/harbor-$release_version-source.tar.gz"
python3 - "$stage" <<'PY'
import hashlib,pathlib,sys
directory=pathlib.Path(sys.argv[1])
with (directory/'SHA256SUMS').open('w') as out:
    for archive in sorted(directory.glob('*.tar.gz')):
        out.write(f'{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n')
PY
mkdir "$release_dir"
cp "$stage/"*.tar.gz "$stage/SHA256SUMS" "$release_dir/"
echo "Release candidates written to $release_dir"
