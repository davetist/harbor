#!/bin/sh
set -eu
runner="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/harbor"
if [ "$#" -eq 2 ] && [ -x "$1" ] && [ -f "$2" ]; then
 exec "$1" install --config "$2"
fi
if [ "${1:-}" = --desktop ]; then
 shift
 exec "$runner" install "$@"
fi
exec "$runner" install --headless "$@"
