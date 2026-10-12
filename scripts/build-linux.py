#!/usr/bin/env python3
"""Build complete Linux installations in Debian and standalone formats."""
import gzip
import hashlib
import io
import lzma
import pathlib
import re
import sys
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]


def tar_bytes(entries, compression):
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w") as archive:
        directories = set()
        for name, contents, mode, link in entries:
            for parent in reversed(pathlib.PurePosixPath(name).parents):
                if str(parent) == '.' or str(parent) in directories:
                    continue
                directory = tarfile.TarInfo('./' + str(parent))
                directory.type = tarfile.DIRTYPE
                directory.mode = 0o755
                archive.addfile(directory)
                directories.add(str(parent))
            info = tarfile.TarInfo(name)
            info.mode = mode
            if link is not None:
                info.type = tarfile.SYMTYPE
                info.linkname = link
                archive.addfile(info)
            else:
                info.size = len(contents)
                archive.addfile(info, io.BytesIO(contents))
    if compression == "xz":
        return lzma.compress(raw.getvalue(), preset=6)
    return gzip.compress(raw.getvalue(), mtime=0)


def ar_member(name, contents):
    header = f"{name:<16}{0:<12}{0:<6}{0:<6}{'100644':<8}{len(contents):<10}`\n"
    return header.encode("ascii") + contents + (b"\n" if len(contents) % 2 else b"")


def build(version, arch, binary, destination):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?", version):
        raise ValueError("Invalid version")
    if arch not in ("amd64", "arm64"):
        raise ValueError("Unsupported Debian architecture")
    payload = pathlib.Path(binary).read_bytes()
    if payload[:4] != b"\x7fELF":
        raise ValueError("Linux packaging needs an ELF runner")
    machine = int.from_bytes(payload[18:20], "little")
    if machine != {"amd64": 62, "arm64": 183}[arch]:
        raise ValueError("Runner architecture does not match package architecture")
    destination = pathlib.Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    deb = destination / f"harbor-{version}-linux-{arch}.deb"
    installer = destination / f"harbor-{version}-linux-{arch}.run"
    if deb.exists() or installer.exists():
        raise ValueError("Package output already exists")
    control = f"""Package: harbor
Version: {version.replace('-', '~', 1)}
Section: net
Priority: optional
Architecture: {arch}
Maintainer: Harbor contributors <harbor@users.noreply.github.com>
Depends: systemd, xdg-utils, zenity, pkexec
Installed-Size: {(len(payload) + 1023) // 1024}
Homepage: https://github.com/davetist/harbor
Description: A self-hosted Urbit ship runner
 Open Harbor from the application menu or run harbor install over SSH.
 It creates your configuration and
 starts a per-user background runner. Ships keep running when the browser
 closes. Removal preserves ship data and configuration.
"""
    controls = [("./control", control.encode(), 0o644, None)]
    for name in ("preinst", "postinst", "prerm"):
        controls.append(("./" + name, (ROOT / "packaging/linux" / name).read_bytes(), 0o755, None))
    data = [
        ("./usr/lib/harbor/harbor", payload, 0o755, None),
        ("./usr/bin/harbor", b"", 0o777, "../lib/harbor/harbor"),
        ("./usr/share/applications/org.harbor.desktop", (ROOT / "packaging/linux/harbor.desktop").read_bytes(), 0o644, None),
        ("./usr/share/icons/hicolor/scalable/apps/harbor.svg", (ROOT / "web/public/harbor-logo.svg").read_bytes(), 0o644, None),
        ("./usr/share/doc/harbor/copyright", (ROOT / "LICENSE").read_bytes(), 0o644, None),
        ("./usr/share/doc/harbor/README.md", (ROOT / "README.md").read_bytes(), 0o644, None),
    ]
    notices = ROOT / "dist/THIRD_PARTY_NOTICES.txt"
    if not notices.is_file():
        raise ValueError("Build third-party notices before packaging")
    data.append(("./usr/share/doc/harbor/THIRD_PARTY_NOTICES.txt", notices.read_bytes(), 0o644, None))
    with deb.open("xb") as output:
        output.write(b"!<arch>\n")
        for name, contents in [("debian-binary", b"2.0\n"), ("control.tar.xz", tar_bytes(controls, "xz")), ("data.tar.xz", tar_bytes(data, "xz"))]:
            output.write(ar_member(name, contents))
    archive = tar_bytes([
        ("./harbor", payload, 0o755, None),
        ("./install.sh", (ROOT / "packaging/linux/install.sh").read_bytes(), 0o755, None),
        ("./uninstall.sh", (ROOT / "packaging/linux/uninstall.sh").read_bytes(), 0o755, None),
        ("./system-install.sh", (ROOT / "packaging/linux/system-install.sh").read_bytes(), 0o755, None),
        ("./harbor.desktop", (ROOT / "packaging/linux/harbor.desktop").read_bytes(), 0o644, None),
        ("./harbor.svg", (ROOT / "web/public/harbor-logo.svg").read_bytes(), 0o644, None),
        ("./LICENSE", (ROOT / "LICENSE").read_bytes(), 0o644, None),
        ("./THIRD_PARTY_NOTICES.txt", notices.read_bytes(), 0o644, None),
    ], "gz")
    checksum = hashlib.sha256(archive).hexdigest()
    header = f"""#!/bin/sh
# Harbor {version}, Linux {arch}. Offline installer; no compiler or network download needed.
set -eu
if [ "${{1:-}}" = --help ]; then
 echo 'usage: sh harbor-*.run [--upgrade] [--config /absolute/config.json]'
 echo 'Installs the harbor command, desktop launcher and background service on desktop or headless Linux.'
 echo 'System installation and startup at boot request administrator approval.'
 exit 0
fi
[ "$(id -u)" -ne 0 ] || {{ echo 'Run this installer as your ordinary user, rather than root.' >&2; exit 1; }}
case "$(uname -m)" in {'x86_64' if arch == 'amd64' else 'aarch64|arm64'}) ;; *) echo 'This installer is for Linux {arch}.' >&2; exit 1;; esac
[ "$(uname -s)" = Linux ] || {{ echo 'This installer requires Linux.' >&2; exit 1; }}
stage="$(mktemp -d)"
trap 'rm -rf -- "$stage"' EXIT HUP INT TERM
line="$(awk '/^__HARBOR_ARCHIVE_BELOW__$/ {{ print NR+1; exit }}' "$0")"
tail -n +"$line" "$0" > "$stage/payload.tar.gz"
printf '%s  %s\\n' '{checksum}' "$stage/payload.tar.gz" | sha256sum -c - >/dev/null
tar -xzf "$stage/payload.tar.gz" -C "$stage"
sh "$stage/install.sh" "$@"
exit 0
__HARBOR_ARCHIVE_BELOW__
"""
    with installer.open("xb") as output:
        output.write(header.encode())
        output.write(archive)
    installer.chmod(0o755)
    return deb, installer


if __name__ == "__main__":
    if len(sys.argv) != 5:
        raise SystemExit("usage: build-linux.py VERSION ARCH BINARY OUTPUT_DIRECTORY")
    for artifact in build(*sys.argv[1:]):
        print(artifact)
