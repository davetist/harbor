# Contributing to Harbor

Harbor aims to make self-hosted Urbit approachable while preserving a quiet, elegant interface. Small fixes, reproducible bug reports, native-platform tests and documentation improvements are welcome. Read the [architecture](docs/architecture.md) and [validation boundary](docs/validation.md) before changing lifecycle behavior.

For runtime catalog updates or launch/mapping changes, also read the [LMDB mapping workaround and retirement criteria](docs/lmdb-mapping-workaround.md). A newer Vere version does not automatically establish that the POSIX loom collision is fixed or that its default map fits supported hosts.

Use Go 1.27.2 or newer, Node 24.x and npm. Install with `npm --prefix web ci`, then run `HARBOR_GO=go scripts/build.sh` before Go tests: the generated embedded UI is deliberately excluded from source control. Production starts with `./dist/harbor serve`; `npm --prefix web run dev` serves the separate simulated visual preview at `/preview.html`.

Before proposing a change, run:

```sh
go test -race ./...
go vet ./...
npm --prefix web run typecheck
npm --prefix web run test
go mod download all
python3 scripts/collect-notices.py go
python3 scripts/test_installation_packages.py
```

For installation changes, rebuild the runner and opt into a disposable OS-service trial with `HARBOR_NATIVE_INSTALL_TEST=1 HARBOR_INSTALL_BINARY="$PWD/dist/harbor" go test -v ./internal/installation -run TestNativeServiceLifecycle`. Supplying `HARBOR_INSTALL_UPGRADE_BINARY` adds a native replacement trial. The test creates a unique service and temporary empty registry, then verifies data-preserving removal. Keep clean-machine download/manual macOS approval, privileged package-manager and actual boot-without-login checks separate. Build the Mac shell with `HARBOR_GO=go sh packaging/macos/build-app.sh VERSION`; see [macOS distribution](docs/macos-distribution.md).

For UI changes, install Chromium into the project cache and run browser checks:

```sh
PLAYWRIGHT_BROWSERS_PATH="$PWD/.tools/playwright" npx --prefix web playwright install chromium
npm --prefix web run test:e2e
```

Report what was tested and on which native platform. Fixture browser tests and cross-compilation do not establish real ship acceptance. Never use a real identity for integration or interruption tests. Native tests are opt-in; inspect their path/flag guards first. Clean up only your disposable fixtures after checking that no process or job uses them. Keep keys, codes, piers, archives, logs, private screenshots and operator paths out of issues and patches.

Preserve exclusive writers, durable recovery holds, exact Roll/Chop dependencies, and the distinction between migration exports and update/replay checkpoints. Add focused regressions for lifecycle bugs. Keep VPNs, tunneling, certificates, mount/formatting, device pairing and public hosting outside Harbor's scope. Accessibility and visual consistency matter alongside functionality.

Use Conventional Commit titles when submitting commits. Explain the concrete user behavior, validation and remaining limits in pull requests. Contributions are provided under the project's MIT license; include required notices for new dependencies. Report vulnerabilities privately using the [security policy](SECURITY.md).
