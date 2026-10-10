#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
(cd web && npm run build -- --outDir ../internal/ui/dist --emptyOutDir)
mkdir -p dist
GOCACHE="$PWD/.tools/gocache" GOPATH="$PWD/.tools/gopath" "${HARBOR_GO:-go}" build -trimpath -o dist/harbor ./cmd/harbor
