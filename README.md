# Harbor

[![Checks](https://github.com/davetist/harbor/actions/workflows/ci.yml/badge.svg)](https://github.com/davetist/harbor/actions/workflows/ci.yml)

Harbor is an open-source, self-hosted Urbit ship runner for **macOS and Linux, on x86_64 and ARM64**. It aims to bring [Port](https://github.com/latter-bolden/port)'s simple, elegant approach back to life: easy onboarding, a quiet interface, and practical tools for looking after your ships.

A native Go service runs your ships. The Mac app and Linux application launcher open the same interface; closing the window or browser leaves ships running. The interface uses upstream Urbit sigils and keeps maintenance, runtime updates, recovery and the Dojo within reach.

![Harbor ship list, shown with simulated preview data](docs/images/ships.png)

*Visual preview with simulated ships; native validation is documented separately.*

**Experimental alpha.** Apple Silicon and Raspberry Pi 4 candidates passed stopped-comet import, all four maintenance commands, export/restore, replay and authenticated restarts. The Mac candidate also passed shutdown during first boot and resumed the preserved pier. Native Intel ship execution, valid identity-key onboarding/migration, physical external-drive failures and broader recovery acceptance remain open. The LMDB workaround mitigates an upstream address collision; an earlier untraced Pi Pack crash remains preserved with its recovery hold. Use disposable ships while these acceptance gates are unfinished. See [validation](docs/validation.md) and [release readiness](docs/release-readiness.md).

## Start Harbor

Installation candidates provide familiar desktop installs and a separate headless path:

- **Mac (macOS 13+):** open the universal `.dmg`, drag Harbor into Applications, then open it. If macOS blocks the unsigned download, [allow Harbor](docs/installation.md#allow-harbor-on-macos). Harbor creates its configuration and background runner. Use the Harbor menu to stop or uninstall it.
- **Debian/Ubuntu/Linux desktop:** open the matching `.deb` in your software installer, then launch **Harbor** from the application menu. The launcher sets up your per-user runner and opens your browser. Its menu actions stop or uninstall Harbor.
- **Headless Linux / Raspberry Pi:** download the matching `.run` and run `sh harbor-VERSION-linux-arm64.run` (use `amd64` on Intel/AMD). It installs offline, configures the service, handles startup without login, and prints the browser addresses. Administrator approval may be requested for startup at boot.

Mac downloads use manual first-launch approval without Developer ID signing or notarization. If macOS blocks Harbor, open **System Settings → Privacy & Security**, choose **Open Anyway** for Harbor, and confirm **Open**. Alternatively, after dragging Harbor into Applications, run:

```sh
xattr -dr com.apple.quarantine "/Applications/Harbor.app"
```

Then open Harbor again. The command removes the download quarantine attribute only from Harbor and its bundled files. A newer download may require approval again; see [macOS installation](docs/installation.md#allow-harbor-on-macos).

Ship data and configuration survive updates and removal. See [installation](docs/installation.md) for updating, advanced configuration and existing-service safeguards. Every push to `main` runs [Build and publish install packages](https://github.com/davetist/harbor/actions/workflows/release-candidate.yml). Each versioned prerelease on [GitHub Releases](https://github.com/davetist/harbor/releases) contains five installers: one universal Mac `.dmg`, Linux `.deb` and `.run` installers for amd64 and arm64, plus `SHA256SUMS` for those five files. Release downloads do not expire; they remain available until explicitly deleted. Each release identifies the commit it was built from. GitHub also supplies its own source ZIP/tar links automatically.

### From source

From a source checkout, install Go 1.27.2 or newer and Node 24.x, then run:

```sh
git clone https://github.com/davetist/harbor.git
cd harbor
npm --prefix web ci
HARBOR_GO=go scripts/build.sh
./dist/harbor serve
```

Open **http://127.0.0.1:18789** on that computer, or **http://HOST-IP:18789** from another device on your trusted LAN or tailnet. Harbor opens **Your ships** when ships exist, and onboarding when the registry is empty. Installed binary packages need neither Node nor a database daemon.

Choose **Start with a key**, **Try a comet**, or **Use an existing pier**. Existing folders belong to the computer running Harbor; uploaded files come from your browser's computer. Stop an existing ship in its previous runner before registering it.

The default Harbor listener is IPv4 `0.0.0.0:18789`; ship HTTP ports start at 8080. Harbor validates Host and browser origins without filtering clients or interfaces by IP range. **There is no separate Harbor login: clients that can reach the control interface can control all ships.** Use a trusted LAN and external firewall or tailnet access rules; see [network configuration](docs/installation.md#network-access). Do not forward these ports publicly.

## What it does

- Boot from a key, create a comet, register a stopped existing pier, or restore a full Harbor archive.
- Start and stop ships, open their web interface, retrieve an access code, and use the Dojo.
- Run Pack, Meld, Roll and Chop, with editable schedules and resource checks.
- Export a verified full pier, update the native runtime, replay, and retain interrupted-work recovery holds.
- Use a mounted external drive, with filesystem identity checks before startup.
- Run as a per-user macOS LaunchAgent or Linux systemd service.

Automatic maintenance starts enabled on the measured hardware/runtime/loom profiles; other profiles start paused with an explanation. Existing schedules keep their settings. See [maintenance](docs/maintenance.md).

Tailscale, mounting/formatting drives, certificates and network policy remain external. Harbor does not configure them.

## Documentation

- [Installation, services and configuration](docs/installation.md)
- [Onboarding, loom size and migration](docs/onboarding.md)
- [Maintenance defaults and resource checks](docs/maintenance.md)
- [External storage](docs/external-storage.md)
- [Retained uploads and export cleanup](docs/retention.md)
- [Native acceptance trials](docs/native-acceptance.md)
- [macOS distribution and first-launch approval](docs/macos-distribution.md)
- [Architecture](docs/architecture.md)
- [LMDB mapping workaround, evidence and retirement criteria](docs/lmdb-mapping-workaround.md)
- [Validation and remaining acceptance gates](docs/validation.md)
- [Publication audit and release procedure](docs/release-readiness.md)
- [Contributing](CONTRIBUTING.md) · [Security policy](SECURITY.md) · [Changelog](CHANGELOG.md)

Report reproducible problems in [GitHub issues](https://github.com/davetist/harbor/issues). Report security vulnerabilities privately using the [security policy](SECURITY.md).

## License

Harbor is licensed under [MIT](LICENSE). Distributions include third-party license notices. The separately downloaded Urbit runtime retains its upstream license.
