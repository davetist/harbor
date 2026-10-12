#!/bin/sh
# System-wide files for the standalone Linux installer; services stay per-user.
set -eu
umask 022
[ "$(id -u)" = 0 ] || { echo 'System installation requires administrator approval.' >&2; exit 1; }
action=install
if [ "${1:-}" = --uninstall ]; then action=uninstall; shift; fi
prefix="${1:-/usr/local}"
case "$prefix" in /*) ;; *) echo 'Installation prefix must be absolute.' >&2; exit 1;; esac
[ "$prefix" != / ] || { echo 'Installation prefix cannot be the filesystem root.' >&2; exit 1; }
case "$prefix" in *'='*|*'%'*|*'\'*|*'"'*|*'`'*|*'$'*|*'
'*) echo 'Installation prefix contains unsupported desktop-launcher characters.' >&2; exit 1;; esac
prefix="$(printf '%s' "$prefix" | sed 's#//*#/#g;s#/$##')"
[ -n "$prefix" ] || { echo 'Installation prefix cannot be the filesystem root.' >&2; exit 1; }
case "$prefix" in */./*|*/../*|*/.|*/..) echo 'Installation prefix must not contain dot path components.' >&2; exit 1;; esac
bundle="$(CDPATH= cd -- "$(dirname "$0")" && pwd -P)"
root="$prefix/lib/harbor"
command="$prefix/bin/harbor"
launcher="$prefix/share/applications/org.harbor.desktop"
icon="$prefix/share/icons/hicolor/scalable/apps/harbor.svg"
license="$prefix/share/doc/harbor/LICENSE"
notices="$prefix/share/doc/harbor/THIRD_PARTY_NOTICES.txt"
record="$root/system.sha256"
if [ "$action" = install ] && [ "$prefix" = /usr/local ] && command -v dpkg-query >/dev/null 2>&1; then
 package_state="$(dpkg-query -W -f='${db:Status-Status}' harbor 2>/dev/null || true)"
 case "$package_state" in ''|not-installed|config-files) ;; *)
  echo 'The Harbor Debian package already owns system files. Upgrade or repair it with your package manager, or remove it before using the standalone installer; ship data is preserved.' >&2
  exit 1;; esac
fi
check_directory() {
 [ ! -L "$1" ] || { echo 'Installation directory is a symlink; preserve it for inspection.' >&2; exit 1; }
 if [ -e "$1" ]; then
  [ -d "$1" ] || { echo 'Installation path is not a directory.' >&2; exit 1; }
  metadata="$(stat -c '%u:%a' "$1")"
  [ "${metadata%%:*}" = 0 ] && [ "$((0${metadata#*:} & 0022))" -eq 0 ] || {
   echo 'System installation requires root-owned directories without group or other write access.' >&2; exit 1;
  }
 fi
}
directory="$prefix"
while :; do
 check_directory "$directory"
 [ "$directory" != / ] || break
 directory="$(dirname "$directory")"
done
for directory in "$prefix" "$prefix/bin" "$prefix/lib" "$root" "$prefix/share" \
 "$prefix/share/applications" "$prefix/share/icons" "$prefix/share/icons/hicolor" \
 "$prefix/share/icons/hicolor/scalable" "$prefix/share/icons/hicolor/scalable/apps" \
 "$prefix/share/doc" "$prefix/share/doc/harbor"; do
 check_directory "$directory"
done
mkdir -p "$prefix/lib"
[ ! -L "$prefix/lib/.harbor-install.lock" ] || { echo 'Unexpected installation lock.' >&2; exit 1; }
exec 9>"$prefix/lib/.harbor-install.lock"
flock -x 9
if [ -e "$record" ] || [ -L "$record" ]; then
 [ -f "$record" ] && [ ! -L "$record" ] || { echo 'Unrecognized system installation record.' >&2; exit 1; }
 [ -L "$command" ] && [ "$(readlink "$command")" = ../lib/harbor/harbor ] || { echo 'Harbor command changed outside the installer.' >&2; exit 1; }
 for file in "$root/harbor" "$root/system-install.sh" "$launcher" "$icon" "$license" "$notices"; do
  [ -f "$file" ] && [ ! -L "$file" ] || { echo 'Harbor system file replaced outside the installer.' >&2; exit 1; }
 done
 sha256sum -c "$record" >/dev/null || { echo 'Harbor system files changed outside the installer; preserve them.' >&2; exit 1; }
else
 [ "$action" = install ] || { echo 'No owned standalone system installation found.' >&2; exit 1; }
 for file in "$command" "$root/harbor" "$root/system-install.sh" "$launcher" "$icon" "$license" "$notices"; do
  [ ! -e "$file" ] && [ ! -L "$file" ] || { echo 'Existing system files need inspection before installation.' >&2; exit 1; }
 done
fi
if [ "$action" = uninstall ]; then
 "$root/harbor" package-maintenance remove
 rm -- "$command" "$launcher" "$icon" "$license" "$notices" "$root/harbor" "$root/system-install.sh" "$record"
 rmdir "$root" "$prefix/share/doc/harbor" 2>/dev/null || true
 exit 0
fi
mkdir -p "$root" "$prefix/bin" "$prefix/share/applications" \
 "$prefix/share/icons/hicolor/scalable/apps" "$prefix/share/doc/harbor"
stage="$(mktemp -d "$root/.install-XXXXXX")"
trap 'rm -rf -- "$stage"' EXIT HUP INT TERM
install -m 0755 "$bundle/harbor" "$stage/harbor"
install -m 0755 "$bundle/system-install.sh" "$stage/system-install.sh"
install -m 0644 "$bundle/harbor.svg" "$stage/harbor.svg"
install -m 0644 "$bundle/LICENSE" "$stage/LICENSE"
install -m 0644 "$bundle/THIRD_PARTY_NOTICES.txt" "$stage/THIRD_PARTY_NOTICES.txt"
desktop_command="$(printf '%s' "$command" | sed 's/\\/\\\\/g; s/"/\\"/g; s/`/\\`/g; s/\$/\\$/g')"
while IFS= read -r line; do
 case "$line" in
  Exec=/usr/bin/harbor*) printf 'Exec="%s"%s\n' "$desktop_command" "${line#Exec=/usr/bin/harbor}" ;;
  *) printf '%s\n' "$line" ;;
 esac
done <"$bundle/harbor.desktop" >"$stage/org.harbor.desktop"
chmod 0644 "$stage/org.harbor.desktop"
mv -f -- "$stage/harbor" "$root/harbor"
mv -f -- "$stage/system-install.sh" "$root/system-install.sh"
mv -f -- "$stage/harbor.svg" "$icon"
mv -f -- "$stage/LICENSE" "$license"
mv -f -- "$stage/THIRD_PARTY_NOTICES.txt" "$notices"
mv -f -- "$stage/org.harbor.desktop" "$launcher"
if [ ! -L "$command" ]; then ln -s ../lib/harbor/harbor "$command"; fi
sha256sum "$root/harbor" "$root/system-install.sh" "$launcher" "$icon" "$license" "$notices" >"$stage/system.sha256"
chmod 0600 "$stage/system.sha256"
mv -f -- "$stage/system.sha256" "$record"
"$root/harbor" package-maintenance upgrade
