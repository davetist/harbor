#!/usr/bin/env python3
"""Exercise package formats and source rebuilding inputs, using their consumers."""
import importlib.util
import io
import pathlib
import subprocess
import tarfile
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("build_linux", ROOT / "scripts/build-linux.py")
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class PackageTests(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory(prefix="harbor-package-test-")
        self.addCleanup(self.work.cleanup)
        self.directory = pathlib.Path(self.work.name)
        self.binary = self.directory / "harbor"
        # Packaging contract fixture; native runner/service trials are separate.
        self.payload = b"\x7fELF" + bytes(14) + (183).to_bytes(2,"little") + bytes(44)
        self.binary.write_bytes(self.payload)

    def test_debian_archive_is_readable_and_contains_launchers_and_notices(self):
        deb, installer = builder.build("0.1.0-alpha.4", "arm64", self.binary, self.directory)
        members = subprocess.check_output(["ar","-t",str(deb)],text=True).splitlines()
        # BSD ar exposes the System V trailing name delimiter; GNU ar removes it.
        self.assertEqual([name.rstrip('/') for name in members],["debian-binary","control.tar.xz","data.tar.xz"])
        control_name = next(name for name in members if name.rstrip('/') == "control.tar.xz")
        data_name = next(name for name in members if name.rstrip('/') == "data.tar.xz")
        control = subprocess.check_output(["ar","-p",str(deb),control_name.rstrip('/')])
        with tarfile.open(fileobj=io.BytesIO(control)) as archive:
            fields = archive.extractfile("./control").read().decode()
            self.assertIn("Architecture: arm64\n",fields)
            self.assertIn("Version: 0.1.0~alpha.4\n",fields)
            self.assertEqual(archive.getmember("./prerm").mode,0o755)
            self.assertEqual(archive.getmember("./postinst").mode,0o755)
        data = subprocess.check_output(["ar","-p",str(deb),data_name.rstrip('/')])
        with tarfile.open(fileobj=io.BytesIO(data)) as archive:
            self.assertEqual(archive.extractfile("./usr/lib/harbor/harbor").read(),self.payload)
            self.assertEqual(archive.getmember("./usr/bin/harbor").linkname,"../lib/harbor/harbor")
            self.assertTrue(archive.extractfile("./usr/share/doc/harbor/THIRD_PARTY_NOTICES.txt").read())
            self.assertEqual(archive.getmember("./usr/share/applications/org.harbor.desktop").mode,0o644)
        help_text = subprocess.check_output(["sh",str(installer),"--help"],text=True)
        self.assertIn("--upgrade",help_text)
        self.assertIn("administrator approval",help_text)

    def test_architecture_mismatch_creates_no_installable_artifact(self):
        with self.assertRaisesRegex(ValueError,"architecture"):
            builder.build("0.1.0", "amd64", self.binary, self.directory)
        self.assertEqual(list(self.directory.glob("*.deb")),[])

    def test_existing_output_is_preserved(self):
        deb, _ = builder.build("0.1.0", "arm64", self.binary, self.directory)
        original = deb.read_bytes()
        with self.assertRaisesRegex(ValueError,"already exists"):
            builder.build("0.1.0", "arm64", self.binary, self.directory)
        self.assertEqual(deb.read_bytes(),original)

    def test_source_archive_contains_agent_guide_and_desktop_build_inputs(self):
        source = self.directory / "source.tar.gz"
        subprocess.run(["python3",str(ROOT / "scripts/build-source.py"),"0.1.0",str(source)],check=True,stdout=subprocess.DEVNULL)
        with tarfile.open(source) as archive:
            names = set(archive.getnames())
            self.assertEqual(archive.extractfile("harbor-0.1.0/AGENTS.md").read(), (ROOT / "AGENTS.md").read_bytes())
            for entry in ["packaging/macos/Harbor.swift","packaging/macos/Info.plist","packaging/macos/Harbor.icns","packaging/macos/build-app.sh","packaging/linux/harbor.desktop","packaging/linux/prerm","packaging/linux/postinst","scripts/build-linux.py","scripts/test_installation_packages.py","internal/installation/installation.go"]:
                self.assertIn("harbor-0.1.0/"+entry,names)
            self.assertFalse(any(".harbor-work" in entry or ".tools" in entry or "node_modules" in entry for entry in names))


if __name__ == "__main__":
    unittest.main()
