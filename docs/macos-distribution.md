# macOS distribution

Harbor's distribution is a universal **Harbor.app** inside a drag-to-Applications disk image. Building requires macOS with Xcode command-line tools, Go 1.27.2+, Node 24.x and Python 3. Installing the built app needs none of these development tools.

Harbor is distributed without Developer ID signing or Apple notarization. First launch requires a manual macOS approval when the download is quarantined.

## Install and allow Harbor

Open the disk image, drag **Harbor** to **Applications**, and open it from Applications. If macOS blocks the first launch, follow [Apple's instructions](https://support.apple.com/en-us/102445): open **System Settings → Privacy & Security**, click **Open Anyway** for Harbor, then confirm **Open**.

Alternatively, after copying your trusted Harbor download to Applications, open Terminal and run:

```sh
xattr -dr com.apple.quarantine "/Applications/Harbor.app"
```

Then open Harbor again. The command removes only the download quarantine attribute from Harbor and its bundled files. If you install the app somewhere else, use its actual path. A newer download may require the approval again. The disk image includes the same instructions in **Install Harbor.txt**.

Harbor sets up its per-user background runner on first launch. Closing the window or quitting the interface leaves ships running; the Harbor menu provides stop and uninstall actions. Configuration and ship data survive updates and removal.

## Build

Replace `VERSION` with an unused release version. The builder writes the app, disk image and checksums to `dist/desktop/VERSION/` and refuses to overwrite an existing output directory.

```sh
HARBOR_GO=go sh packaging/macos/build-app.sh VERSION
```

The builder compiles Intel/Apple Silicon Go runners and AppKit/WebKit shells, combines them with `lipo`, includes the existing Harbor icon and license notices, and creates the `.dmg` with an Applications shortcut and first-launch instructions. Output directories are immutable. The Mac app registers the same per-user LaunchAgent used by the CLI installer; the ship runner lives outside the app bundle and survives window close or interface quit.

The executable and bundle retain local **ad-hoc signatures** for Apple Silicon execution and integrity checks. These use no signing certificate, Apple account, keychain identity or notarization service, and do not identify Harbor as an Apple-verified download.

Before publication, independently download the exact final disk image on a clean Mac, verify its checksum, drag into Applications, and test the documented manual approval, first launch, background permission, file upload/download, window-close independence, replacement upgrade and removal with retained data. Local developer-machine success is separate from this clean-download check.

The published [`0.1.0-main.2` prerelease](https://github.com/davetist/harbor/releases/tag/v0.1.0-main.2) passed a [fresh public Safari download and quarantine-command trial](validation.md#public-mac-download-and-allowance--2026-10-11) on an Apple Silicon Mac with an existing empty installation. Its checksum matched the release manifest, Finder preserved Safari's quarantine, macOS blocked the downloaded app before the command, and the same app opened successfully afterward. The runner upgraded from `install.8` while retaining configuration and registrations, and native Browse opened correctly. Earlier local-download trials of [install.8](validation.md#latest-mac-download-and-allowance--2026-10-11) and [install.4](validation.md#macos-browser-download-and-allowance--2026-10-11) are also recorded. Clean-Mac first installation and the **Open Anyway** alternative remain separate checks.
