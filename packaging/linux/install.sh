#!/bin/sh
set -eu
# Unprivileged per-user service. The supplied paths must be absolute.
if [ "$#" -ne 2 ]; then echo 'usage: install.sh /absolute/harbor /absolute/config.json' >&2; exit 2; fi
case "$1:$2" in *[!A-Za-z0-9_./:-]*) echo 'Choose service paths using letters, numbers, dots, slashes, hyphens or underscores.' >&2; exit 2;; esac
case "$1" in /*) ;; *) exit 2;; esac
case "$2" in /*) ;; *) exit 2;; esac
[ -x "$1" ] && [ -f "$2" ]
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$unit_dir"
unit_path="$unit_dir/harbor.service"
if [ -e "$unit_path" ]; then echo 'Harbor unit already exists; inspect it before replacing.' >&2; exit 1; fi
sed -e "s|HARBOR_BINARY|$1|g" -e "s|HARBOR_CONFIG|$2|g" -e "s|HARBOR_DIRECTORY|$(dirname "$2")|g" "$(dirname "$0")/harbor.service" > "$unit_path"
chmod 600 "$unit_path"
systemctl --user daemon-reload
systemctl --user enable --now harbor.service
echo 'Harbor is installed for this user. Piers and configuration remain in the supplied locations.'
echo 'For boot without an interactive session, your administrator must explicitly enable user lingering.'
