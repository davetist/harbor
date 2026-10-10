#!/bin/sh
set -eu
unit_path="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/harbor.service"
[ -f "$unit_path" ] || { echo 'No Harbor user unit installed.'; exit 0; }
grep -q '^Description=Harbor Urbit ship runner$' "$unit_path" || { echo 'Unrecognized unit; refusing removal.' >&2; exit 1; }
systemctl --user stop harbor.service
systemctl --user disable harbor.service
rm -- "$unit_path"
systemctl --user daemon-reload
echo 'Harbor service removed. Piers, backups and configuration preserved.'
echo 'Check for surviving Vere writers before using another runner.'
