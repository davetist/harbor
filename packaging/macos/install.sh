#!/bin/sh
set -eu
if [ "$#" -ne 2 ]; then echo 'usage: install.sh /absolute/harbor /absolute/config.json' >&2; exit 2; fi
case "$1:$2" in *[\<\>\&\"\']*) echo 'Choose paths without XML metacharacters.' >&2; exit 2;; esac
case "$1" in /*) ;; *) exit 2;; esac
case "$2" in /*) ;; *) exit 2;; esac
[ -x "$1" ] && [ -f "$2" ]
case "$2" in "$HOME/Desktop/"*|"$HOME/Documents/"*|"$HOME/Downloads/"*) echo 'Place the background config and data under Library/Application Support, outside macOS protected folders.' >&2; exit 2;; esac
service_home="$HOME/Library/Application Support/Harbor"
mkdir -p "$service_home/bin"
chmod 700 "$service_home" "$service_home/bin"
installed_binary="$service_home/bin/harbor"
if [ -e "$installed_binary" ]; then
 cmp -s "$1" "$installed_binary" || { echo 'An installed Harbor binary differs; preserve it and perform an explicit upgrade before registering this agent.' >&2; exit 1; }
else
 cp "$1" "$installed_binary"
 chmod 700 "$installed_binary"
fi
agent_path="$HOME/Library/LaunchAgents/org.harbor.runner.plist"
[ ! -e "$agent_path" ] || { echo 'Harbor agent already exists; inspect before replacing.' >&2; exit 1; }
mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs/Harbor"
chmod 700 "$HOME/Library/Logs/Harbor"
cat > "$agent_path" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>org.harbor.runner</string>
<key>ProgramArguments</key><array><string>$installed_binary</string><string>serve</string><string>--config</string><string>$2</string></array>
<key>StandardOutPath</key><string>$HOME/Library/Logs/Harbor/service.log</string>
<key>StandardErrorPath</key><string>$HOME/Library/Logs/Harbor/error.log</string>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
<key>AbandonProcessGroup</key><true/>
<key>ExitTimeOut</key><integer>0</integer>
<key>Umask</key><integer>63</integer>
</dict></plist>
PLIST
chmod 600 "$agent_path"
plutil -lint "$agent_path"
launchctl bootstrap "gui/$(id -u)" "$agent_path"
echo 'Harbor starts in your login session. Configuration and piers use the supplied paths.'
