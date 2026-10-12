#!/usr/bin/env python3
"""Exercise package formats and source rebuilding inputs, using their consumers."""
import importlib.util
import io
import os
import pathlib
import shutil
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
            self.assertEqual(archive.getmember("./preinst").mode,0o755)
        data = subprocess.check_output(["ar","-p",str(deb),data_name.rstrip('/')])
        with tarfile.open(fileobj=io.BytesIO(data)) as archive:
            preceding = set()
            for member in archive.getmembers():
                for parent in pathlib.PurePosixPath(member.name).parents:
                    if str(parent) != '.':
                        self.assertIn('./' + str(parent), preceding, 'dpkg needs parent directories before their files')
                if member.isdir():
                    preceding.add(member.name.rstrip('/'))
            self.assertEqual(archive.extractfile("./usr/lib/harbor/harbor").read(),self.payload)
            self.assertEqual(archive.getmember("./usr/bin/harbor").linkname,"../lib/harbor/harbor")
            self.assertTrue(archive.extractfile("./usr/share/doc/harbor/THIRD_PARTY_NOTICES.txt").read())
            self.assertEqual(archive.getmember("./usr/share/applications/org.harbor.desktop").mode,0o644)
        help_text = subprocess.check_output(["sh",str(installer),"--help"],text=True)
        self.assertIn("--upgrade",help_text)
        self.assertIn("administrator approval",help_text)

    def test_run_contains_the_same_cli_and_desktop_integration(self):
        _, installer = builder.build("0.1.0", "arm64", self.binary, self.directory)
        payload = installer.read_bytes().split(b'__HARBOR_ARCHIVE_BELOW__\n', 1)[1]
        with tarfile.open(fileobj=io.BytesIO(payload), mode='r:gz') as archive:
            for name in ('system-install.sh', 'harbor.desktop', 'harbor.svg'):
                self.assertIsNotNone(archive.extractfile('./' + name))

    def test_standalone_install_provides_cli_and_launcher_and_preserves_foreign_files(self):
        bundle = self.directory / 'bundle'
        bundle.mkdir()
        for name in ('install.sh', 'system-install.sh', 'harbor.desktop'):
            shutil.copy2(ROOT / 'packaging/linux' / name, bundle / name)
        shutil.copy2(ROOT / 'web/public/harbor-logo.svg', bundle / 'harbor.svg')
        for name in ('LICENSE', 'THIRD_PARTY_NOTICES.txt'):
            (bundle / name).write_text('test notice\n')
        runner = bundle / 'harbor'
        runner.write_text('#!/bin/sh\nprintf "%s\\n" "$@" > "$HARBOR_TEST_ARGUMENTS"\n')
        runner.chmod(0o755)
        commands = self.directory / 'commands'
        commands.mkdir()
        for name, contents in {
            'sudo': '#!/bin/sh\nHARBOR_TEST_UID=0; export HARBOR_TEST_UID\nexec "$@"\n',
            'id': '#!/bin/sh\nprintf "%s\\n" "${HARBOR_TEST_UID:-1000}"\n',
            'sha256sum': '#!/bin/sh\nexec shasum -a 256 "$@"\n',
            # This portable fixture tests file integration. Native Linux trials
            # exercise the real flock and administrator boundary separately.
            'flock': '#!/bin/sh\nexit 0\n',
            'stat': '#!/bin/sh\nif [ "$3" = "${HARBOR_TEST_UNSAFE_DIRECTORY:-}" ]; then echo 1000:755; else echo 0:755; fi\n',
            'dpkg-query': '#!/bin/sh\necho "${HARBOR_TEST_DPKG_STATUS:-installed}"\n',
            'mkdir': '#!/bin/sh\ncase "$2" in /usr/local*|/usr//local*) echo "fixture refuses global writes" >&2; exit 77;; esac\nexec /bin/mkdir "$@"\n',
        }.items():
            path = commands / name
            path.write_text(contents)
            path.chmod(0o755)
        prefix = (self.directory / 'system with spaces').resolve()
        arguments = self.directory / 'arguments'
        env = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ['PATH'],
                   HARBOR_INSTALL_PREFIX=str(prefix), HARBOR_TEST_ARGUMENTS=str(arguments))
        subprocess.run(['sh', str(bundle / 'install.sh'), '--upgrade'], env=env, check=True, capture_output=True)
        cli = prefix / 'bin/harbor'
        self.assertTrue(cli.is_symlink())
        self.assertEqual(cli.resolve().read_bytes(), runner.read_bytes())
        launcher = prefix / 'share/applications/org.harbor.desktop'
        self.assertIn('Exec="' + str(cli) + '" desktop\n', launcher.read_text())
        self.assertTrue((prefix / 'share/icons/hicolor/scalable/apps/harbor.svg').is_file())
        self.assertEqual(arguments.read_text().splitlines(), ['install', '--upgrade'])
        subprocess.run(['sh', str(bundle / 'install.sh'), '--desktop', '--upgrade'], env=env, check=True, capture_output=True)
        self.assertEqual(arguments.read_text().splitlines(), ['install', '--upgrade'])
        subprocess.run(['sh', str(bundle / 'system-install.sh'), '--uninstall', str(prefix)],
                       env=dict(env, HARBOR_TEST_UID='0'), check=True, capture_output=True)
        self.assertEqual(arguments.read_text().splitlines(), ['package-maintenance', 'remove'])
        self.assertFalse(cli.exists())
        self.assertFalse(launcher.exists())
        unsafe = subprocess.run(['sh', str(bundle / 'system-install.sh'), str(prefix)],
                                env=dict(env, HARBOR_TEST_UID='0', HARBOR_TEST_UNSAFE_DIRECTORY=str(prefix.parent)),
                                capture_output=True)
        self.assertNotEqual(unsafe.returncode, 0, 'user-owned ancestor must be refused')
        self.assertFalse(cli.exists())
        for system_prefix in ('/usr/local', '/usr/local/', '/usr//local'):
            for state in ('installed', 'unpacked', 'half-configured'):
                other_format = subprocess.run(['sh', str(bundle / 'system-install.sh'), system_prefix],
                                              env=dict(env, HARBOR_TEST_UID='0', HARBOR_TEST_DPKG_STATUS=state),
                                              capture_output=True)
                self.assertNotEqual(other_format.returncode, 0)
                self.assertIn(b'Debian package', other_format.stderr)
        subprocess.run(['sh', str(bundle / 'install.sh'), '--upgrade'], env=env, check=True, capture_output=True)
        original = launcher.read_text() + '# user change\n'
        launcher.write_text(original)
        result = subprocess.run(['sh', str(bundle / 'install.sh'), '--upgrade'], env=env, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(launcher.read_text(), original)

    def test_debian_install_refuses_shadowing_standalone_files(self):
        root = self.directory / 'dpkg-root'
        env = dict(os.environ, DPKG_ROOT=str(root))
        command = ['sh', str(ROOT / 'packaging/linux/preinst'), 'install']
        subprocess.run(command, env=env, check=True, capture_output=True)
        launcher = root / 'usr/local/share/applications/org.harbor.desktop'
        launcher.parent.mkdir(parents=True)
        launcher.write_text('operator integration\n')
        result = subprocess.run(command, env=env, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'standalone', result.stderr)
        self.assertEqual(launcher.read_text(), 'operator integration\n')

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
            for entry in ["packaging/macos/Harbor.swift","packaging/macos/Info.plist","packaging/macos/Harbor.icns","packaging/macos/build-app.sh","packaging/linux/harbor.desktop","packaging/linux/system-install.sh","packaging/linux/preinst","packaging/linux/prerm","packaging/linux/postinst","scripts/build-linux.py","scripts/test_installation_packages.py","internal/installation/installation.go"]:
                self.assertIn("harbor-0.1.0/"+entry,names)
            self.assertFalse(any(".harbor-work" in entry or ".tools" in entry or "node_modules" in entry for entry in names))


if __name__ == "__main__":
    unittest.main()
