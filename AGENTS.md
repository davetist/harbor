# Harbor agent guide

This file applies to the whole repository. Harbor is an experimental, single-user
Urbit ship manager for macOS and Linux on amd64 and arm64. Windows is unsupported.
Use this guide to install a development copy, find the relevant code, and debug
without putting an operator's ships at risk. Read current source when behavior
differs from documentation; do not treat past validation as fresh evidence.

The first named public alpha is [v0.1.0-alpha.1](https://github.com/davetist/harbor/releases/tag/v0.1.0-alpha.1).
Read its [release notes](docs/releases/0.1.0-alpha.1.md) and the current
[validation limits](docs/validation.md) before choosing a real-ship workflow.
Earlier `alpha.*` names in validation records are local candidates; GitHub's
`main.*` prereleases are automated development builds.

## Working here

- Be direct, friendly, and practical. Carry requested changes through verification.
- Before editing, inspect `git status`, applicable agent instructions,
  [CONTRIBUTING.md](CONTRIBUTING.md), and the implementation and tests you will touch.
  Preserve unrelated changes and keep fixes surgical.
- For diagnosis or review, investigate before changing state. For a bug fix,
  establish a useful reproduction and add a focused regression when practical.
- Keep installation, ship runtime operations, and Urbit identity management
  separate. A runtime failure is not a reason to breach an identity.
- Use disposable state for development and interruption tests. Never use a real
  identity key, run a second copy of a networked identity, or operate on a live pier
  without authorization covering that action.
- Do not put keys, access codes, cookies, private piers, archives, raw logs, or
  personal paths/addresses in commits, issues, screenshots, or tool output.
- Do not create branches, commits, pushes, or releases unless requested or part
  of the agreed workflow. Use Conventional Commits when committing. Read
  `.github/pull_request_template.md` and check for overlapping work before a PR.

## Install Harbor

### Agent installation runbook

1. Confirm the requested host, OS/CPU (`uname -sm`), service user, installation
   versus upgrade, and whether a Harbor service or another pier writer already
   exists. Linux must be 64-bit with systemd; a 32-bit Pi OS needs an OS upgrade,
   not the ARM64 installer. Windows and non-systemd Linux are unsupported.
2. Prefer the named alpha linked above for this release. Inspect its actual assets;
   do not use GitHub's `/releases/latest` API for a prerelease, guess an asset URL,
   or select an automated `main.*` build merely because it sorts newer.
   `gh release view v0.1.0-alpha.1 --repo davetist/harbor` is an optional way to
   inspect it; `gh` is not an installation prerequisite. Download the matching
   installer and that release's `SHA256SUMS` into a new private folder.
3. Verify the exact selected file before opening or running it. If downloading
   only one installer, select its checksum line rather than asking the checker
   to verify the other four absent files:

   ```sh
   # Set this to the exact downloaded filename for this host.
   harbor_installer=Harbor-0.1.0-alpha.1-macos-universal.dmg
   awk -v artifact="$harbor_installer" '$2 == artifact {print; found=1} END {if (!found) exit 1}' SHA256SUMS > selected-SHA256SUMS
   shasum -a 256 -c selected-SHA256SUMS  # macOS
   # Linux: sha256sum -c selected-SHA256SUMS
   ```

   Require a successful checksum result; a missing entry is a failure.
4. Install using the platform path below. Run the app and `harbor install` as
   the intended ordinary user. Use administrator authority only for the
   documented package/system-file or lingering steps. Do not run Harbor or Vere
   as root, install a second manager, or replace a foreign service manually.
5. Verify the installed runner with `harbor service status --json`. On Mac, use
   `/Applications/Harbor.app/Contents/Resources/harbor` when there is no CLI on
   PATH. Read the reported configuration, runner, running/enabled state and URL;
   use that runner's `version` command and inspect private fields locally.
   Request the reported URL's `/api/v1/host` and `/api/v1/ships`, then open its
   onboarding or ship list. An empty registry is expected only on a fresh installation.
6. For ship acceptance, use **Try a comet** in an explicitly disposable folder,
   wait for the durable onboarding job to complete, then check running state,
   **Open ship**, a fresh authenticated login, stop and restart. Harbor HTTP 200
   alone does not prove a ship runs. Use the guarded native trial below for
   repeatable lifecycle validation. Real keys/piers require their own authorization.

Replacement through the CLI/package path preserves configuration and service
startup intent. Use `sudo apt install ./ACTUAL.deb`, the newer `.run --upgrade`,
or the newer runner's `install --upgrade`, as appropriate to the original
installation. Opening the Mac app or Linux desktop launcher explicitly starts
Harbor, including a deliberately stopped manager. To upgrade a stopped Mac
installation while keeping it stopped, copy the new app into Applications and
run its bundled runner's `install --upgrade` without opening the app. Inspect
status before deciding to start it. Switching `.deb`/`.run`
formats requires the documented data-preserving removal first, with authorization
for that service interruption. See [updates and removal](docs/installation.md#updates-and-removal).

### Choose the appropriate installation

Check the host OS/CPU and any existing installation first. Obtain the matching
installer and `SHA256SUMS` from [Harbor releases](https://github.com/davetist/harbor/releases),
and verify the chosen file's SHA-256 before running it. Do not guess a current
release number or overwrite an existing service to resolve an ownership refusal.
See [installation](docs/installation.md) for updates and legacy-service migration.

| Host | Installation |
| --- | --- |
| macOS 13+ | Open the universal `.dmg`, drag Harbor to Applications, and open it. The app installs the background runner and opens its interface. |
| Debian/Ubuntu / 64-bit Raspberry Pi OS | As the ordinary user, run `sudo apt install ./ACTUAL.deb` with the verified matching `amd64` or `arm64` file, then launch Harbor from the application menu or run `harbor install` over SSH. `apt` resolves package dependencies. |
| Other systemd Linux distributions | As the ordinary service user, run `sh harbor-VERSION-linux-arm64.run`; substitute `amd64` on Intel/AMD. Both formats supply the CLI and desktop launcher. Use the actual downloaded filename. |
| Source development | Follow the build and isolated foreground setup below. |

The current Mac distribution has no Developer ID signing or notarization and may require
**System Settings → Privacy & Security → Open Anyway** on first launch; follow
[the macOS approval instructions](docs/installation.md#allow-harbor-on-macos).
Linux packages need a working systemd user session. The `.deb` depends on
xdg-utils, Zenity, and pkexec; the `.run` needs those tools for desktop actions.
Both formats enable user lingering for boot without login and may request
administrator approval. The `.run` also requests approval for its system files.
The `.run` requires `sudo`, `runuser`, `flock`, `sha256sum`, and a working user
systemd session; install its desktop tools separately if they are absent.
Installed packages need neither Node nor Go nor a database daemon. Initial ship boot requires
network access to the official Vere release and bootstrap pill hosts.
Initial bootstrap can take several minutes; check durable job state before
declaring a stall. A canceled request does not establish that its writer exited.

Closing the Mac window or browser leaves the background runner and ships running.
`harbor service status --json` inspects an installed service; `start`, `stop`, and
`uninstall` manage it. If `harbor` is not on PATH, use the actual runner executable.
Service removal preserves configuration, SQLite, piers, and exports. Installation
and upgrades must use Harbor's ownership checks, not manual binary replacement.

### Build from source

Use the Go version required by `go.mod` (currently 1.27.2 or newer), Node 24.x, and
npm. Python 3 is needed for packaging and acceptance scripts. From a fresh clone:

```sh
git clone https://github.com/davetist/harbor.git
cd harbor
npm --prefix web ci
HARBOR_GO=go scripts/build.sh
./dist/harbor version
```

In an existing checkout, skip cloning. If Go is outside PATH, set `HARBOR_GO` to
its executable path for build scripts and use that same executable for Go checks.
The build uses repository-local `.tools/gocache` and `.tools/gopath` caches.

`scripts/build.sh` builds React into `internal/ui/dist` and embeds it in
`dist/harbor`. Build before Go tests on a clean checkout: `internal/ui/embed.go`
requires that generated directory. A plain `npm --prefix web run build` writes
`web/dist`, which does not refresh the embedded production interface. Rebuild the
runner after UI changes; generated output and local state are ignored by Git.

### Run an isolated development instance

Plain `./dist/harbor serve` uses the operator's default data directory and can
resume registered ships. For debugging, create a private, empty registry and bind
to loopback instead. From the repository root:

```sh
umask 077
harbor_trial="$(mktemp -d "${TMPDIR:-/tmp}/harbor-dev-XXXXXX")"
cat > "$harbor_trial/config.json" <<EOF
{
  "dataDir": "$harbor_trial/data",
  "listenAddress": "127.0.0.1:0",
  "defaultShipHTTPBindAddress": "127.0.0.1"
}
EOF
./dist/harbor serve --config "$harbor_trial/config.json"
```

Open the actual URL printed at startup; port `0` assigns a free control port.
An empty registry should show onboarding and `GET /api/v1/ships` should return an
empty list. Configuration paths must be absolute; JSON does not expand `~` or
environment variables, and unknown fields are rejected. Ctrl-C requests graceful
shutdown; wait for the runner, owned ships, and utilities to exit before removing
only the disposable directory you created. Preserve failed trials for diagnosis.

For visual development, `npm --prefix web run dev` serves
`http://127.0.0.1:5173/preview.html` with simulated ships and localStorage.
The production entry at `/` uses the API; Vite's `/api` proxy expects a runner on
`127.0.0.1:18789`, so configure that fixed port for API-backed Vite work after
checking it is free. The preview is useful for layout but cannot prove ship boot,
maintenance, login, or migration behavior.

## Architecture and data flow

Harbor is one Go service with an embedded React/TypeScript interface. The Mac
AppKit/WebKit shell and Linux desktop launcher use the same service. SQLite holds
manager state; each pier's Urbit state is separate and uses Vere's LMDB/event log.

Browser actions reach `/api/v1`, which persists lifecycle requests as jobs.
The executor claims the durable queue and uses the supervisor to stop/start
owned ships or run offline utilities. The scheduler adds due maintenance to that
same queue. Server-sent events at `/api/v1/events` trigger UI refreshes; durable
API/SQLite state remains authoritative after reconnects.

| Location | Responsibility / debugging entry point |
| --- | --- |
| `cmd/harbor/main.go`, `internal/config` | CLI, configuration, volume/controller guards, wiring, listeners, graceful shutdown. |
| `internal/app/startup.go` | Observe prior processes, reconcile interrupted jobs, then queue permitted automatic starts. |
| `internal/installation`, `cmd/harbor/installation.go` | Managed runner install, service ownership, status, upgrade rollback, removal. |
| `packaging/macos`, `packaging/linux` | Native Mac shell, Linux launcher/installers, OS service integration. |
| `internal/server` | API routes, Host/origin checks, onboarding, folders, recovery, events, terminal. |
| `internal/onboarding` | Key/comet boot, stopped existing-pier registration, uploads, archive restore. |
| `internal/jobs` | Serialized lifecycle/maintenance/export/replay/runtime-update jobs and durable recovery phases. |
| `internal/supervisor` | Vere child ownership/readiness, port allocation, PTY input/output, graceful stop. |
| `internal/runtime` | Pinned official Vere catalog/digests, native binary checks, arguments, LMDB sizing, private HTTP control. |
| `internal/store`, `internal/model` | SQLite migrations/transactions, ships, jobs, schedules, process records, logs, shared types. |
| `internal/scheduler` | Calendar/DST scheduling, measured resource profiles, maintenance ordering and Roll/Chop pairing. |
| `internal/pier`, `internal/storage`, `internal/backup` | Canonical paths, exclusive writers, mount/UUID checks, verified full-pier archives. |
| `internal/retention` | Explicit retained-upload/export accounting and guarded pruning. |
| `internal/updates` | Harbor release discovery/version comparison; distinct from Vere runtime updates. |
| `web/src/api`, `web/src/model.ts`, `web/src/screens` | API client/events/types and real UI flows; `web/src/preview` owns simulated data. |
| `scripts`, `.github/workflows` | Build/source/release packaging, notices, fixture checks and opt-in native acceptance. |

Without explicit configuration, `dataDir` is
`~/Library/Application Support/Harbor` on macOS and `$XDG_CONFIG_HOME/Harbor` or
`~/.config/Harbor` on Linux. It contains `harbor.sqlite`, `runtimes/`, `uploads/`,
`backups/`, and the default `piers/`. Registered piers may be elsewhere.
`allowedRoots` is a legacy default-folder/volume-pin setting, not a filesystem
whitelist; current path behavior is defined by `internal/config` and `internal/pier`.
The folder browser reads the runner host's directories with its OS user's access.

The default control listener is IPv4 `0.0.0.0:18789`. Ship starts try HTTP port 80,
then 8080 upward; stopped ships do not reserve ports, so a restart can change one.
`advertisedHost` admits the configured hostname; Open links use the browser's
Harbor hostname/IP. It does not change bindings. Existing ships retain their saved
HTTP binding when defaults change. Harbor has no separate login: every reachable
control client can manage all ships. Host/origin checks are not authentication and
do not protect ship HTTP listeners. Keep access on trusted LANs/tailnets with
external firewall policy; do not add public forwarding or bypass those checks.
If accessing Harbor through a DNS, mDNS or Tailscale hostname, set
`advertisedHost` to that exact hostname (without scheme, port or path) in the
existing configuration and perform an authorized graceful service restart.
Otherwise use the actual destination IPv4 address; an unconfigured hostname can
be refused by Host validation. Granting privileged-port capability is a separate
operator action; ordinary installs fall back from port 80 without changing permissions.

## Lifecycle rules to preserve

- **One writer per pier.** Preserve canonical-path validation, Harbor guards, Vere
  locks, and process launch identity checks. Stop a previous runner before import.
  Never delete a lock, adopt an external writer, or signal a PID merely because its
  name looks like Vere. Data directory locking also prevents two Harbor managers.
- **Recovery is durable.** Startup observes and reconciles before admitting work.
  Uncertain launches, interrupted utilities, and failed restarts retain holds.
  Use the supported recovery verification flow after investigating; do not clear
  database flags or repeatedly restart to hide a failure.
- **Shutdown waits.** Preserve PTYs, process tracking, and database access until
  utilities and owned runtimes finish. A canceled request or elapsed timeout does
  not prove a writer stopped. Do not replace graceful shutdown with forced kills.
- **Migration exports leave the source stopped.** Full export persists
  `DesiredRunning=false` before stopping/writing. Keep that intent through failure,
  controller restart, and scheduled maintenance to avoid duplicate live identities.
- **Replay and updates differ.** Replay requires a verified full-pier export and
  creates a fresh checkpoint. A Vere update verifies the candidate before stopping,
  uses the current runtime's `prep`, retains the old executable/checkpoints, and
  restarts only a previously running ship. It does not require a prior full export.
  Harbor's release checker does not automatically install Harbor updates.
- **Maintenance follows measured policy.** Preserve resource checks before Stop
  and again before utility launch, automatic Pack → conditional Meld → Roll → Chop
  ordering, and Chop's dependency on the successful Roll from its exact cycle.
  Manual Chop is a separate standalone operation. Unmeasured profiles start
  automatic schedules paused; existing schedules retain their saved settings.
- **LMDB sizing is a mitigation.** Read `docs/lmdb-mapping-workaround.md` before
  catalog, argument, or launch changes. The current catalog pins Vere 4.6. Ship
  starts and Pack/Meld/Roll/Chop use a power-of-two virtual map of at least 64 GiB
  and twice the largest logical LMDB file; epoch sizes are not summed. `play` and
  `prep` do not accept that flag in 4.6. This does not repair corruption or prove
  upstream address ownership; do not retire it just because the version increased.
- **Storage failures preserve evidence.** Required mount/UUID checks precede
  directory creation and SQLite opening. Never create replacement data on an absent
  drive, truncate an LMDB file, discard history, or prune held checkpoints to make
  a start pass. Harbor does not mount/format drives or configure swap/VPNs.

## Debugging workflow

1. Record the exact Harbor version/commit, OS/CPU, runtime version, loom exponent,
   operation, and error with secrets and personal paths redacted. Distinguish a
   foreground dev process from the installed runner; rebuilding `dist/harbor` does
   not replace a running service or its copied executable.
2. Inspect installed service state with `harbor service status --json`. Linux:
   `systemctl --user status harbor.service` and
   `journalctl --user -u harbor.service`. Mac: `launchctl print gui/$(id -u)/org.harbor.runner`,
   the app's Show Service Logs action, or `~/Library/Logs/Harbor`. Logs/status are
   private evidence; inspect locally and report only the relevant redacted parts.
3. Start with read-only API state: `/api/v1/host`, `/api/v1/ships`,
   `/api/v1/ships/{id}/jobs`, `/api/v1/jobs/{id}`, and
   `/api/v1/jobs/{id}/logs`. Compare desired running state, actual state, job phase,
   recorded launches, and recovery reason. An action's `202`/`jobId` means queued,
   not successfully completed. Avoid direct SQLite writes.
4. For reachability, check process/listener, the printed URL, actual destination,
   Host/Origin, and external network policy. For UI state, inspect API failures,
   SSE reconnect/resync, and whether embedded assets were rebuilt. Keep protections
   enabled while reproducing the issue.
   Open checks the saved ship listener with a bounded public login request. A
   running process can still be busy or unresponsive. Its recovery dialog offers
   a confirmed Ctrl+C for the owned running ship, after rechecking responsiveness,
   pending work, recovery holds and process identity. Never interrupt a real
   ship's computation without operator authorization; a timeout alone does not
   prove it is stuck. See [web recovery](docs/onboarding.md#when-open-does-not-respond).
5. For boot/maintenance, inspect runtime digest/architecture, argument order, loom,
   actual allocated HTTP port, canonical pier path, writer ownership, job output,
   resource gates, and LMDB selection. For storage refusal, confirm the expected
   mount/UUID and OS permissions before touching files.
6. Reproduce with an isolated registry/disposable comet, then trace the relevant
   route → store/job → executor/supervisor/runtime path. Fix the cause, retain
   useful failure evidence, and exercise the same user workflow after the change.
   Do not infer authenticated login from a landing page or public ship name.

## Verification and delivery

After the embedded UI is built, routine code-change checks are:

```sh
go test -race -p 1 ./...
go vet ./...
npm --prefix web run typecheck
npm --prefix web run test
python3 scripts/test_native_acceptance.py
go mod download all
python3 scripts/collect-notices.py go
python3 scripts/test_installation_packages.py
```

Run Go test packages serially (`-p 1`) because runtime helpers in different
packages share preferred HTTP ports during probe/launch. Keep the race detector
and all packages enabled; package parallelism can create fixture port collisions.

Run focused regressions first, then the applicable routine/CI checks; consult
`.github/workflows/ci.yml` for platform builds and dependency advisories. For a
documentation-only change, verify references and instructions; runtime/native
tests apply when the change affects those behaviors. Keep packaging allowlists
current when adding required source files: `scripts/build-source.py` controls
the custom source archive and must include this guide.

For UI changes, install the project-cached browser and run browser checks:

```sh
PLAYWRIGHT_BROWSERS_PATH="$PWD/.tools/playwright" npx --prefix web playwright install chromium
npm --prefix web run test:e2e
```

Render and inspect the changed flow at desktop/mobile sizes and test real
interaction. Fixture browser checks are separate from production/native trials;
`HARBOR_BROWSER_URL`, `HARBOR_ACCEPTANCE_URL`, and folder-browser test variables
must point only at explicitly prepared disposable services. Read the selected
test's guards before opting in; skipped tests do not establish live acceptance.

For lifecycle changes, follow [native acceptance](docs/native-acceptance.md):

```sh
python3 scripts/native-acceptance.py \
  --binary dist/harbor --output /absolute/new-trial-directory \
  --disposable --require-platform linux/amd64 --lifecycle
```

Use the actual native platform (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, or
`linux/arm64`) and a new output directory. This requires official runtime/pill
access and boots a real disposable comet. `--lifecycle` covers Pack/Roll/Chop,
export/restore, replay, and login; Meld, runtime transitions, physical faults,
installer/OS startup, and real-identity migration need separate relevant trials.
Inspect `harbor_chaos` and environment guards before interruption tests.

Installation changes also require the disposable OS-service trial described in
`CONTRIBUTING.md` (`HARBOR_NATIVE_INSTALL_TEST=1` and an absolute
`HARBOR_INSTALL_BINARY`). Mac shell builds use
`HARBOR_GO=go sh packaging/macos/build-app.sh VERSION`. Package download approval,
privileged package management, and boot without login are separate acceptance
checks; service fixtures alone cannot establish them.

Before reporting completion, inspect the final diff and state, finish temporary
resource cleanup where safe, and state what was tested on which platform. Report
remaining applicable checks as incomplete with their real blocker. Cross-builds,
preview screenshots, and helper-process tests do not establish native ship
acceptance; current evidence and open gates live in `docs/validation.md` and
`docs/release-readiness.md`.

## Further references

- [Architecture](docs/architecture.md) and [security boundary](SECURITY.md).
- [Onboarding, loom, and migration](docs/onboarding.md).
- [Maintenance](docs/maintenance.md), [external storage](docs/external-storage.md),
  and [retention](docs/retention.md).
- [LMDB workaround and retirement gates](docs/lmdb-mapping-workaround.md).
- [Native acceptance](docs/native-acceptance.md), [macOS distribution](docs/macos-distribution.md),
  and [release readiness](docs/release-readiness.md).
- [Runtime-update evidence](docs/validation/runtime-updates.md) and
  [bootstrap/shutdown evidence](docs/validation/bootstrap-shutdown.md).

For upstream Urbit procedures, start with the public
[Urbit skills index](https://urbit.org/.agents/skills/index.md) and
[running Urbit OS](https://urbit.org/.agents/skills/running-urbit-os/SKILL.md).
Check upstream guidance for the runtime actually in use; Harbor's supported
catalog, arguments, ownership, and recovery policy still govern this repository.
