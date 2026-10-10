# Installation and configuration

Harbor builds for macOS and Linux on x86_64 (`amd64`) and ARM64 (`arm64`). Native acceptance currently covers Apple Silicon and Raspberry Pi 4; Intel artifacts are cross-built. Windows is unsupported.

## Foreground operation

Extract the binary archive for the host's OS/CPU, check it against `SHA256SUMS`, and run `./harbor version`. Then start `./harbor serve`. Source builds are described in the README. Keep the service process running; closing its browser is safe, ending the foreground process stops Harbor and its owned ships gracefully.

Without a configuration file, data lives in `Harbor` under the OS user configuration directory: `~/Library/Application Support/Harbor` on macOS, and `$XDG_CONFIG_HOME/Harbor` or `~/.config/Harbor` on Linux. Default permitted piers live in `dataDir/piers`.

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
| `allowedRoots` | Permitted host folders for piers; defaults to `dataDir/piers`. This is a path policy, not an OS sandbox. |
| `timezone` | Host IANA timezone; choose explicitly if detection is unsuitable. |
| `maxArchiveBytes` | 16 GiB compressed uploads; configurable up to 64 GiB. Extraction has a separate fixed 64 GiB limit. |
| `requiredVolumes` | Mount path + filesystem UUID pins; see external storage. |

## Network access

On the service host, open `http://127.0.0.1:18789`. Other devices use `http://LAN-IP:18789` or the host's Tailscale IPv4 address. Tailscale must already be installed and its policy must allow access; Harbor performs no VPN setup. The default IPv4 wildcard follows newly connected LAN/Tailscale interfaces without a Harbor restart.

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

Binary archives include the appropriate `install.sh` and `uninstall.sh`. Supply an executable and an existing configuration file, both absolute paths:

```sh
sh ./install.sh /absolute/path/to/harbor /absolute/path/to/config.json
```

On macOS, keep configuration/data under `~/Library/Application Support/Harbor`, outside Desktop/Documents/Downloads. The installer copies the binary into that location and registers `org.harbor.runner` in your login session. Logs are under `~/Library/Logs/Harbor`. Signing/notarization is unfinished; a downloaded binary may need explicit macOS approval. No signed `.app` is provided.

On Linux, the installer registers `harbor.service` with user systemd. Paths must use letters, numbers, dots, slashes, hyphens or underscores. Inspect it with `systemctl --user status harbor.service`; read logs with `journalctl --user -u harbor.service`. It starts with the user session. Boot without login requires an administrator to enable lingering explicitly. The installer does not do that for you.

Installers refuse existing service registrations or a differing installed macOS binary. Upgrades are currently manual: stop ships and Harbor, preserve configuration/data, unregister the service, install the chosen executable deliberately, then register again. On macOS, `uninstall.sh` preserves the installed binary as well as data; a differing replacement needs an explicit file replacement. No automatic service-binary updater is provided. Runtime updates inside Harbor update Vere, not Harbor.

Run the platform's `sh ./uninstall.sh` to unregister Harbor. Removal preserves piers, archives, configuration and database. Inspect retained data separately; uninstalling is not secure erasure.

## Troubleshooting

An unreachable page usually means the process is stopped, the bind address is unsuitable, or external network policy blocks the port. Check service status and the startup URL before changing configuration. A recovery hold needs its displayed recovery action; repeatedly restarting Harbor does not remove it. A missing pinned drive causes startup refusal before replacement data is created. An existing writer must stop before registration or offline operations.

## Assigned ports and storage locations

A `listenAddress` port of `0` uses the assigned listener port for Host/origin validation; read the actual URL from the service startup line. The host API reports manager state in `dataDir`, the default pier destination in `pierRoot`, and all permitted destinations in `allowedRoots`. Onboarding uses `pierRoot`. These locations can be on different filesystems.

See [retention](retention.md) for explicit cleanup/accounting and [macOS signing](macos-signing.md) for the prepared Developer ID installer build.
