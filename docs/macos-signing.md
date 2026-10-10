# Signed macOS distribution

The source checkout includes a universal signed/notarized `.pkg` build path. It requires existing **Developer ID Application** and **Developer ID Installer** identities in the macOS keychain, plus an authenticated `notarytool` keychain profile. Apple Development certificates do not satisfy these distribution requirements. Configure the profile privately using Apple's notarization tools; no Apple password or API key belongs in source, shell history or Harbor configuration.

```sh
HARBOR_GO=go sh packaging/macos/build-signed.sh 0.1.0-alpha.2 \
  'Developer ID Application: Your Organization (TEAMID)' \
  'Developer ID Installer: Your Organization (TEAMID)' \
  YOUR_EXISTING_NOTARY_PROFILE
```

The script checks credentials before creating output, builds Intel/ARM binaries, combines them with `lipo`, applies hardened-runtime signing with a timestamp, creates a signed installer, submits it for notarization, requires `Accepted`, staples the ticket, and verifies the stapled installer with Gatekeeper. Only after those checks does it write the installer, notarization result and checksum to a new `dist/signed/VERSION/` directory. It refuses an existing version and never publishes or changes the keychain.

The installer contains the universal CLI, license/notices, documentation and service-install scripts under `/usr/local/lib/harbor`. It does not register/start a service or overwrite pier state/configuration automatically. After reviewing/installing the package, use `/usr/local/lib/harbor/bin/harbor` with the [ordinary service setup](installation.md). An application wrapper and a Harbor self-updater remain separate work.

A prepared script, ad-hoc signature or development certificate is not proof of signed distribution acceptance. Before release, build with the actual Developer ID credentials, install the exact downloaded/stapled package on a clean Mac, verify Gatekeeper acceptance and native operation, and check deliberate binary replacement with the service stopped. Signing/notarization and clean-machine installation remain open until those trials pass.
