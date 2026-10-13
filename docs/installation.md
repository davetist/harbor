# Installation and configuration

Harbor builds for macOS and 64-bit systemd Linux on x86_64 (`amd64`) and ARM64 (`arm64`). Fresh disposable-comet lifecycle acceptance passed on all four targets for the first named alpha; native installer evidence is broader on Apple Silicon and Raspberry Pi 4 than on Intel. See [current validation](validation.md#first-named-alpha-release-pass--2026-10-13) for the exact boundary. Windows, 32-bit Linux and non-systemd Linux are unsupported.

## Desktop installation

On **macOS 13 or later**, open the universal Harbor disk image, drag **Harbor** to **Applications**, and open it. Harbor is distributed without Developer ID signing or notarization; if macOS blocks the first launch, [allow Harbor](#allow-harbor-on-macos) using the steps below. The app creates a default configuration, installs a private runner, registers its login service, and opens the existing Harbor interface in a native window. If macOS disables background activity, enable Harbor in System Settings and open it again.

Closing the window or choosing **Quit Harbor Interface** leaves the runner and ships running. The Harbor menu shows runner state and provides **Stop Harbor**, **Show Ship Data**, **Show Service Logs** and **Uninstall Harbor**. Stop waits for ongoing operations and graceful ship shutdown. Open Harbor to start it again. Uninstall removes the service and copied runner, then moves the app to the Trash; configuration, piers and exports remain.

In the Mac app, **Browse** opens the normal macOS folder browser. In a web browser, including the Linux desktop launcher's browser, it opens Harbor's directory browser for the machine running Harbor. Open folders, go up to parents, use breadcrumbs or Home, or enter an absolute path and press Go. Hidden folders can be shown. **Use this folder** confirms the current directory; Cancel or Escape keeps the existing form value. Listings come from the host filesystem using the service user's OS permissions. Import and boot accept any accessible, writable destination while preserving canonical-path, symlink, overwrite, storage-pin and exclusive-writer checks. `allowedRoots` supplies legacy defaults and startup volume-pin paths; it is not a folder whitelist. Selecting another folder does not require changing that list or restarting Harbor. See [external storage](external-storage.md) when the destination needs a pinned mount.

On Linux, both `.deb` and `.run` install the same command, application-menu launcher, and background runner. Choose `.deb` for Debian, Ubuntu, or Raspberry Pi OS and `.run` for other systemd distributions. Both work over SSH and on a desktop; no desktop/headless edition must be selected.

After installing a `.deb` matching your CPU through the software installer or `sudo apt install ./harbor-VERSION-linux-arm64.deb`, open **Harbor** from the application menu or run `harbor install` as your ordinary user. Substitute `amd64` on Intel/AMD. Setup creates the configuration and user service, enables startup at boot, and opens your default browser when launched graphically. `harbor desktop` opens the same interface later. Closing the browser leaves ships running. The launcher's **Stop Harbor and ships** and **Uninstall Harbor** actions use graphical confirmation. Package installation/removal uses the OS administrator prompt; ship data remains in the original user's folders. The `.deb` depends on systemd, xdg-utils, Zenity and pkexec. No RPM/AppImage is supplied.

The Mac candidate can be built with `HARBOR_GO=go sh packaging/macos/build-app.sh VERSION`; the complete release build also produces Linux `.deb` and `.run` installers. See [macOS distribution](macos-distribution.md) for build details and download acceptance.

### Allow Harbor on macOS

After attempting to open Harbor, go to **System Settings → Privacy & Security**, click **Open Anyway** for Harbor, then confirm **Open**. This is [Apple's manual approval procedure](https://support.apple.com/en-us/102445).

For the terminal alternative, copy your trusted Harbor download to Applications first, then run:

```sh
xattr -dr com.apple.quarantine "/Applications/Harbor.app"
```

Open Harbor again. This removes the download quarantine attribute only from Harbor and its bundled files. If you chose another installation folder, use its actual app path. The disk image's **Install Harbor.txt** includes these instructions; a newer download may require approval again.

## Standalone Linux installer

Download the `.run` matching the machine (`arm64` for a 64-bit Pi, `amd64` for Intel/AMD), then run this one installation command as your ordinary user:

```sh
sh harbor-VERSION-linux-arm64.run
```

The installer verifies its embedded payload and installs a system-owned runner under `/usr/local/lib/harbor`, the `harbor` command under `/usr/local/bin`, and the desktop launcher, icon, and license notices under `/usr/local/share`. It then creates the default configuration, copies the private runner, registers and starts a user systemd service, and prints the local and trusted-network browser addresses. It needs no compiler, Node, database daemon or installation-time network download. The host needs systemd with a normal user/SSH login session, sudo, runuser, flock, and sha256sum. Desktop launch requires a browser, xdg-utils and Zenity; graphical removal also requires pkexec. The installer does not add a desktop environment to an SSH-only machine.

System file installation requests administrator approval through sudo. Both Linux formats enable user lingering through `loginctl` during user setup so Harbor starts at boot and survives logout, requesting administrator approval if needed. Setup fails clearly if that approval cannot be obtained; it does not change sudoers or authentication. Lingering remains enabled on removal because other user services may depend on it. The old `--desktop` installer flag is accepted for compatibility and uses the same setup. Initial SSH access and external network policy remain the operator's responsibility.

Use one Linux format at a time. Before switching, remove the previous system installation using the removal command below; it stops its services and retains configuration and ship data. Then install the other format and run `harbor install` or open Harbor. The installers refuse conflicting integration instead of leaving an older command or launcher ahead of the new one. Standalone system directories and their ancestors must belong to root and must not grant other users write access.

## Foreground operation

Binary archives are advanced-use outputs of the local `scripts/build-release.sh VERSION` builder; the public GitHub release publishes the installers above and their checksums. If using a locally built binary archive, extract the host's OS/CPU version, check it against that build's `SHA256SUMS`, and run `./harbor version`. Then start `./harbor serve`. Source builds are described in the README. Keep the service process running; closing its browser is safe, ending the foreground process stops Harbor and its owned ships gracefully.

Without a configuration file, data lives in `Harbor` under the OS user configuration directory: `~/Library/Application Support/Harbor` on macOS, and `$XDG_CONFIG_HOME/Harbor` or `~/.config/Harbor` on Linux. The default pier destination is `dataDir/piers`; you can choose any other accessible folder.

To choose locations, save a private JSON file and pass its absolute path:

```json
{
  "dataDir": "/absolute/private/harbor-data",
  "timezone": "Europe/Madrid"
}
```

```sh
./harbor serve --config /absolute/private/config.json
```

JSON does not expand `~` or environment variables. Unknown fields and relative storage paths are rejected. The service user must be able to write the data/pier folders and their parents, where Harbor creates lock files. Protect the configuration and data from other users.

| Setting | Default / purpose |
| --- | --- |
| `listenAddress` | `0.0.0.0:18789`, Harbor control interface. |
| `defaultShipHTTPBindAddress` | `0.0.0.0`, IPv4 web binding for new registrations. Existing ships keep their saved binding. |
| `advertisedHost` | Optional hostname/IP for **Open ship**. Otherwise use the browser's Harbor hostname/IP. This does not change listeners. |
| `dataDir` | Manager database, runtimes, uploads and backups. |
| `allowedRoots` | Legacy default-folder list; defaults to `dataDir/piers`. The first folder is suggested during setup. Also supplies paths for optional startup volume pins; does not restrict where piers can be stored. |
| `timezone` | Host IANA timezone; choose explicitly if detection is unsuitable. |
| `maxArchiveBytes` | 16 GiB compressed uploads; configurable up to 64 GiB. Extraction has a separate fixed 64 GiB limit. |
| `requiredVolumes` | Mount path + filesystem UUID pins; see external storage. |

## Network access

On the service host, open `http://127.0.0.1:18789`. Other devices use `http://LAN-IP:18789` or the host's Tailscale IPv4 address. Tailscale must already be installed and its policy must allow access; Harbor performs no VPN setup. The default IPv4 wildcard follows newly connected LAN/Tailscale interfaces without a Harbor restart.

For a DNS, mDNS or Tailscale hostname, set `advertisedHost` to that exact host
without scheme, port or path, then gracefully restart the service. An
unconfigured hostname can be refused even when it resolves to the right machine;
use the destination IPv4 address while diagnosing it.

Each ship chooses its HTTP port when it starts: Harbor tries port 80 first, then 8080, 8081 and upward, skipping occupied ports and ports reserved by ships currently starting or running. If the OS refuses port 80 without extra privileges, Harbor falls back without requesting them. Stopped ships do not reserve ports, and a ship's port may change after a restart. **Open ship** uses the currently selected port.

Harbor does not filter clients or destination interfaces by IP range. With a wildcard listener, Host must match the actual local destination address or the configured `advertisedHost`; loopback destinations also accept `localhost`. With an explicit listener, Host must match the configured listener host or `advertisedHost`; loopback listeners also accept localhost aliases. Browser mutations check Origin and `Sec-Fetch-Site`. These request checks are not authentication. A client that can reach the control interface can retrieve codes, export piers, stop ships and use the Dojo.

The configured listener and external firewall/tailnet policy determine which devices can connect. Keep router forwarding/public proxies off and enforce that external policy for both Harbor and ship HTTP ports. Ship HTTP listeners have their own runtime behavior and are not protected by Harbor's Host and origin checks. The default Harbor listener is IPv4 only.

For local-only operation set both fields to loopback:

```json
{
  "listenAddress": "127.0.0.1:18789",
  "defaultShipHTTPBindAddress": "127.0.0.1",
  "dataDir": "/absolute/private/harbor-data"
}
```

This affects new registrations; existing ships retain saved bindings. There is no certificate, tunneling, discovery or device-pairing service in Harbor.

## Background service

Binary archives also include `install.sh` and `uninstall.sh`. On Mac, `sh ./install.sh` creates defaults and installs the user service. On Linux it supplies the same system command, desktop launcher, and boot service as the `.run`. The direct command `./harbor install` installs only that binary's user service; Linux setup enables boot without login by default. Advanced source users can opt out of lingering with `--headless=false`.

To keep an existing advanced configuration, pass its absolute path:

```sh
./harbor install --config /absolute/path/to/config.json
```

On macOS, default configuration/data is under `~/Library/Application Support/Harbor`, the copied runner and installation record are in its `runner/` directory, and the installer registers `org.harbor.runner` in your login session. Logs are under `~/Library/Logs/Harbor`. Keep background configuration/data outside protected Desktop/Documents/Downloads folders. The app copies its runner out of the app bundle, so moving or closing the app does not invalidate a running service.

On Linux, the copied runner and installation record live in `~/.local/share/Harbor/runner`; the default configuration/data follows `$XDG_CONFIG_HOME/Harbor` or `~/.config/Harbor`. The installer registers `harbor.service` with user systemd. Spaces and Unicode paths are quoted; no shell evaluates service arguments. Inspect it with `systemctl --user status harbor.service`; read logs with `journalctl --user -u harbor.service`.

An installation record and exact service/binary checks protect ownership. An unrelated, legacy or externally modified registration is refused before replacement. Existing installations made by the older scripts require deliberate migration: stop and unregister the old service using its original tooling, preserve its runner/configuration/data, then install the new runner with the same `--config` path. The new installer does not guess ownership or adopt a surviving writer.

## Updates and removal

The bottom-right footer shows the running Harbor version as a button. Clicking it shows **Checking for updates…** and refreshes immediately. When a newer installable release is found, that same label shows the available version in light yellow and opens [Harbor's GitHub releases](https://github.com/davetist/harbor/releases) when clicked. Harbor also checks when the interface opens, on focus and hourly; background requests share a six-hour service cache. Prerelease installations can see newer prereleases; stable installations are offered stable releases. Development builds and unrecognized version strings show their version and the latest release without claiming to be outdated. Failed background checks stay quiet, and a known update remains available during a failed recheck.

Update checks run through the Harbor service and require access to GitHub's public API. They use no credentials or ship information. The version link leads to the existing installation procedure below; it does not replace the running binary automatically.

On Mac, replace the app in Applications with the newer download and open it; repeat the [macOS approval](#allow-harbor-on-macos) if requested. Harbor gracefully stops its installed runner if its bundled version changed, replaces the copied executable, and starts it with the same configuration. Opening the Mac app or Linux desktop launcher intentionally starts the manager. To keep a deliberately stopped Mac manager stopped after replacement, run `/Applications/Harbor.app/Contents/Resources/harbor install --upgrade` without opening the app. CLI replacement preserves startup intent. On Debian/Ubuntu, a package upgrade updates only per-user installations originating from that package. Deliberately stopped services remain stopped; logged-out users retain their next-login startup preference. A failed candidate startup attempts restoration of the previous runner. Interrupted/uncertain replacements preserve the previous executable and fail closed for inspection.

For a standalone Linux installation, run the newer `.run` with `--upgrade`, or `sh ./install.sh --upgrade` from a newer Linux binary archive. System installation updates only user services originating from that exact system runner. For a direct binary's user service, run the newer binary's `harbor install --upgrade`. There is no network self-updater; runtime updates inside Harbor update Vere, rather than the Harbor application.

Advanced service controls:

```sh
harbor service status
harbor service stop
harbor service start
harbor service uninstall
```

`harbor service uninstall` removes only the current user's service and private runner. To remove Linux system files as well, use the launcher action, remove a `.deb` through your software installer, or run `sudo sh /usr/local/lib/harbor/system-install.sh --uninstall` for a standalone installation. The Linux archive's `sh ./uninstall.sh` also removes its system installation. System removal stops and removes only registrations originating from that exact system runner, including those of logged-out users. Other-source installations are preserved. Removal preserves piers, archives, configuration and database. Inspect retained data separately; uninstalling is not secure erasure.

## Troubleshooting

An unreachable page usually means the process is stopped, the bind address is unsuitable, or external network policy blocks the port. Check service status and the startup URL before changing configuration. A recovery hold needs its displayed recovery action; repeatedly restarting Harbor does not remove it. A missing pinned drive causes startup refusal before replacement data is created. An existing writer must stop before registration or offline operations.

## Assigned ports and storage locations

A `listenAddress` port of `0` uses the assigned listener port for Host/origin validation; read the actual URL from the service startup line. The host API reports manager state in `dataDir`, the default pier destination in `pierRoot`, and legacy default/volume-pin paths in `allowedRoots`. Onboarding suggests `pierRoot` and accepts other accessible destinations. These locations can be on different filesystems.

See [retention](retention.md) for explicit cleanup/accounting and [macOS distribution](macos-distribution.md) for app/disk-image builds and first-launch approval.
