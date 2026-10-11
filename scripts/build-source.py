#!/usr/bin/env python3
"""Create a reproducible, allowlisted source archive without private/generated state."""
import gzip
import pathlib
import re
import tarfile
import sys

root = pathlib.Path(__file__).resolve().parents[1]
version = sys.argv[1] if len(sys.argv) > 1 else "0.1.0-alpha.1"
if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?", version):
    raise SystemExit("Invalid version")
destination = pathlib.Path(sys.argv[2]) if len(sys.argv) > 2 else root / f"dist/harbor-{version}-source.tar.gz"
if destination.exists():
    raise SystemExit("Source archive already exists; inspect rather than overwrite")
files = [root / name for name in ("README.md", "LICENSE", "CONTRIBUTING.md", "SECURITY.md",
                                "CHANGELOG.md", ".gitignore", "go.mod", "go.sum")]
groups = {
    "cmd": {".go"}, "internal": {".go"},
    "packaging": {".sh", ".service", ".swift", ".plist", ".desktop", ".icns"}, "docs": {".md", ".png"},
    ".github": {".md", ".yml", ".yaml"}, "web/src": {".ts", ".tsx", ".css"},
    "web/e2e": {".ts"}, "web/public": {".svg"},
}
for folder, suffixes in groups.items():
    files.extend(p for p in (root / folder).rglob("*") if p.is_file() and p.suffix in suffixes)
files.extend(root / "packaging/linux" / name for name in ("postinst", "prerm"))
# Include only the approved sanitized public evidence and its documented helper.
files.extend(root / "docs/upstream" / name for name in (
    "vere-loom-collision-evidence.json", "vere-loom-collision-core.txt",
    "vere-loom-collision-trace.txt", "vere-loom-pty.py"))
# Machine-bound measurement probes remain private lab tooling.
files.extend(root / "scripts" / name for name in ("build.sh", "build-release.sh",
                                                "build-source.py", "collect-notices.py", "native-acceptance.py",
                                                "test_native_acceptance.py"))
files.extend(root / "scripts" / name for name in ("build-linux.py", "test_installation_packages.py"))
files.extend(root / "web" / name for name in ("package.json", "package-lock.json", "tsconfig.json",
                                            "vite.config.ts", "vitest.config.ts", "playwright.config.ts",
                                            "index.html", "preview.html"))
destination.parent.mkdir(parents=True, exist_ok=True)
with destination.open("xb") as output:
    with gzip.GzipFile(filename="", fileobj=output, mode="wb", mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w") as archive:
            for source in sorted(set(files)):
                relative = source.relative_to(root)
                if source.is_symlink() or not source.is_file():
                    raise SystemExit(f"Not a regular source file: {relative}")
                if "node_modules" in relative.parts or "dist" in relative.parts:
                    raise SystemExit(f"Generated file in source allowlist: {relative}")
                info = tarfile.TarInfo(f"harbor-{version}/{relative.as_posix()}")
                info.size = source.stat().st_size
                info.mode = 0o755 if source.suffix == ".sh" or source.name in ("postinst", "prerm") else 0o644
                with source.open("rb") as content:
                    archive.addfile(info, content)
print(destination)
