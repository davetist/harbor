#!/bin/sh
set -eu
runner="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/harbor"
if [ "$#" -eq 2 ] && [ -x "$1" ] && [ -f "$2" ]; then
 exec "$1" install --config "$2"
fi
if [ "${1:-}" = --desktop ]; then
 # Legacy flag: desktop and SSH installs now use the same integration.
 shift
fi
prefix="${HARBOR_INSTALL_PREFIX:-/usr/local}"
sudo sh "$(dirname "$runner")/system-install.sh" "$prefix"
exec "$prefix/bin/harbor" install "$@"
