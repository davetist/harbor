#!/bin/sh
set -eu
agent_path="$HOME/Library/LaunchAgents/org.harbor.runner.plist"
[ -f "$agent_path" ] || { echo 'No Harbor login agent installed.'; exit 0; }
[ "$(/usr/libexec/PlistBuddy -c 'Print :Label' "$agent_path")" = org.harbor.runner ] || { echo 'Unrecognized agent; refusing removal.' >&2; exit 1; }
launchctl bootout "gui/$(id -u)" "$agent_path"
rm -- "$agent_path"
echo 'Harbor login agent removed. Piers, backups and configuration preserved.'
echo 'Check for surviving Vere writers before using another runner.'
