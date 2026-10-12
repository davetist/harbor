#!/bin/sh
set -eu
directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
if [ -f "$directory/system-install.sh" ]; then
 exec sudo sh "$directory/system-install.sh" --uninstall "${HARBOR_INSTALL_PREFIX:-/usr/local}"
fi
exec "$directory/harbor" service uninstall
