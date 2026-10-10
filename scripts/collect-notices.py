#!/usr/bin/env python3
"""Collect installed Go and production npm license texts; fail on missing notices."""
import json
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
go = sys.argv[1] if len(sys.argv) > 1 else "go"


def output(*args, cwd=root):
    return subprocess.check_output(args, cwd=cwd, text=True)


def licenses(directory, prefixes):
    return sorted(p for p in directory.iterdir()
                  if p.is_file() and p.name.upper().startswith(prefixes))


parts = ["Harbor is licensed under MIT; see LICENSE.\n"
         "Third-party notices for the Go module graph and production browser dependencies.\n"
         "Vere is downloaded separately and retains its upstream license.\n"]
raw = output(go, "list", "-m", "-json", "all")
decoder = json.JSONDecoder()
while raw.strip():
    module, index = decoder.raw_decode(raw.lstrip())
    raw = raw.lstrip()[index:]
    if module.get("Main"):
        continue
    directory = module.get("Dir")
    if not directory:
        raise SystemExit(f"Missing module source: {module['Path']}; run go mod download all first")
    texts = licenses(pathlib.Path(directory), ("LICENSE", "COPYING", "NOTICE"))
    if not texts:
        raise SystemExit(f"Missing license text: {module['Path']}")
    for path in texts:
        parts.append(f"\n--- Go {module['Path']} {module.get('Version', '')} / {path.name} ---\n"
                     + path.read_text(errors="replace"))

# Include nested production packages rather than maintaining a brittle list.
for line in output("npm", "ls", "--omit=dev", "--all", "--parseable", cwd=root / "web").splitlines():
    directory = pathlib.Path(line)
    if directory == root / "web":
        continue
    package = json.loads((directory / "package.json").read_text())
    texts = licenses(directory, ("LICENSE", "COPYING", "NOTICE", "OFL"))
    if not texts:
        raise SystemExit(f"Missing license text: npm {package['name']}")
    for path in texts:
        parts.append(f"\n--- npm {package['name']} {package['version']} / {path.name} ---\n"
                     + path.read_text(errors="replace"))

goroot = pathlib.Path(output(go, "env", "GOROOT").strip())
parts.append("\n--- Go standard library ---\n" + (goroot / "LICENSE").read_text())
(root / "dist").mkdir(exist_ok=True)
(root / "dist/THIRD_PARTY_NOTICES.txt").write_text("\n".join(parts))
