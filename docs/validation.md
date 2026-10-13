# Validation

Evidence reviewed on 2026-10-13 for the first named public alpha. Native trials use isolated disposable fake ships or fresh comets, never an operator's existing pier or identity key. Cross-built artifacts and simulated browser tests are recorded separately from native acceptance. Older dated sections describe their own candidates, not a repeat of those checks on the public alpha.

| Target | Evidence | Remaining |
| --- | --- | --- |
| macOS ARM64 | M2 Max/32 GiB: native Vere boot/login/Dojo, full export/restore, 4.5→4.6 update, full replay, three maintenance passes, LaunchAgent graceful utility shutdown, confirmed/uncertain Pack recovery windows. Fresh-comet browser Dojo/Open and desktop/mobile accessibility. Combined alpha.3 candidate: early-bootstrap shutdown/resume and supplemental stopped-comet import, all four maintenance commands, export/restore/checkpoint replay and authenticated restarts. | Valid disposable-key onboarding, real-identity migration, representative active ships, broader first-boot failure windows. |
| Linux ARM64 | Raspberry Pi 4/8 GB: native service/UI, Mac export restore/login, 4.5→4.6 update/replay, maintenance and due schedule, systemd graceful utility shutdown, Pack interruption holds and fresh restored-folder login. Shared map policy: isolated folder import, repeated/automatic/post-Pack restart and oversized-import refusal. Combined alpha.3 candidate: supplemental stopped-comet import, all four maintenance commands, export/restore/checkpoint replay and authenticated restarts; actual mappings checked. | Physical-drive disconnect/power-loss, valid disposable keys, real identities, larger active ships and broader failure/recovery windows. A separate untraced Pack crash remains held with unresolved cause. |
| macOS x86_64 | Hosted native Go checks and the 2026-10-13 fresh-comet boot/login, stop/start, Pack/Roll/Chop, export/restore/replay and graceful shutdown trial. | Native installer execution, Meld, valid disposable keys, real identities and broader workload/failure acceptance. |
| Linux x86_64 | Hosted native Go checks and the 2026-10-13 fresh-comet boot/login, stop/start, Pack/Roll/Chop, export/restore/replay and graceful shutdown trial. | Native installer execution, Meld, valid disposable keys, real identities and broader workload/failure acceptance. |

## First named alpha release pass — 2026-10-13

The [four-platform native lifecycle run](https://github.com/davetist/harbor/actions/runs/38387194232)
passed on `darwin/arm64`, `darwin/amd64`, `linux/arm64` and `linux/amd64`.
Each credential-free report was downloaded and checked: fresh comet onboarding,
new-cookie authenticated login, stop/start, Pack/Roll/Chop, stopped-source
export, upload/restore, the restored registration's own checkpoint, replay and
final graceful manager exit all passed. This run used source revision
`796b39dbff5ea13ce54410dda758f703dfa82062`; the alpha release preparation changes
documentation only. These were real native comets, not cross-builds or retained
onboarding fixtures. The workflow does not cover Meld, runtime transitions,
valid identity keys, OS installation or physical fault injection.

The locally compiled `0.1.0-alpha.1` Apple Silicon runner independently passed
the same full fresh-comet lifecycle with service exit 0. Its binary SHA-256 was
`824784fe8e41c6922bec6d564d0daeed67de8026b0cdc01b860f3a2633022874`.
A unique temporary Mac LaunchAgent passed installation, idempotent opening,
replacement with the alpha runner, stop/start, preservation of stopped intent
through CLI replacement, and removal with configuration/data retained. The
ordinary installed Harbor service was not replaced by these trials.

The actual CGO-disabled packaged Mac ARM64 alpha runner separately passed fresh
comet boot/login, stop/start, Pack/Roll/Chop, export/restore/replay and graceful
exit. Its SHA-256 was
`b9ab23dc9ddf601e66d5efbc5c1d64087d3f14807ae91c897946bcc64f9c9b16`.
All ten local release-builder outputs passed their checksum manifest; archive
inspection confirmed notices, executable modes, the agent guide in the source
archive and exclusion of private/generated state. The universal Mac disk image
passed read-only inspection of both architectures, ad-hoc signatures, release
version, macOS 13 minimum, Applications shortcut, instructions and notices.
This is local package validation, separate from clean-Mac download/approval.

An independently extracted allowlisted source archive installed from its
lockfile, rebuilt the embedded UI and runner, and passed fresh Go race/vet,
frontend type/unit and native-login contract checks. Its empty-registry runner
served HTTP 200, an empty ship list and production assets byte-identical to the
inspected native browser build, then exited 0. Subsequent release edits changed
documentation only; the final source archive was checked against that tested
implementation and current documentation.

Release checks passed: a fresh `go test -race -count=1 -p 1 ./...`, `go vet ./...`,
frontend type checking and three unit tests, nine native-login contract tests,
seven installation-package tests, module download and third-party notices.
`npm audit` found zero advisories; `govulncheck` v1.8.0 found no vulnerabilities.
The production frontend was rebuilt before Go verification.

All 66 browser tests were exercised on macOS ARM64: 61 routine fixtures passed,
then the five normally guarded native tests passed on newly prepared disposable
services. Native checks covered real host folder browsing, update discovery
against GitHub and current-version refresh, production navigation, Dojo output,
Open ship, Roll/Chop schedule editing, accessibility and desktop/mobile layout.
The ship was a newly bootstrapped offline fake `~zod`; update-version fixtures
advertised the alpha and current public `main.13` versions to cover both states.
All fixture ships stopped and managers/bootstrap processes exited 0. These
browser checks do not establish clean-machine installation or human
screen-reader/physical-touch acceptance.

### Earlier manual observations

The Pi's virgin 4.5 fixture bootstrap needed a retry with a smaller upstream LMDB mapping after an out-of-memory failure. Later ordinary Harbor starts/update/replay passed. That fixture setup is not default onboarding acceptance. Successful paths do not establish every power-loss, forced-kill or filesystem-failure outcome.

Private HTTP access-code login/privacy and deliberate early HTTP disconnects passed on native Mac/Pi. The former socket reader is confined to tests after an upstream early-close crash was reproduced; production has no socket fallback. Recovery verification releases confirmed utility exits to persistent Stopped, while unknown outcomes remain held.

Storage guards passed mounted/missing/replacement APFS image cases on macOS and real kernel mount/UUID/missing/unmounted/wrong-UUID cases on the Pi. No fallback state was created. These checks do not certify USB disconnects, enclosure/power failures or actual external-drive performance.

The demo's LAN/Tailscale HTTP reachability, host-correct **Open ship**, enabled user service and graceful restart were verified. Reboot behavior was not exercised. Browser-close independence is provided by service architecture; foreground termination still stops owned ships.

## Automated checks

Routine commands:

```sh
npm --prefix web ci
HARBOR_GO=go scripts/build.sh
go test -race -p 1 ./...
go vet ./...
python3 scripts/test_native_acceptance.py
npm --prefix web run typecheck
npm --prefix web run test
PLAYWRIGHT_BROWSERS_PATH="$PWD/.tools/playwright" npx --prefix web playwright install chromium
npm --prefix web run test:e2e
```

The Go race suite, vet, TypeScript check and frontend unit suite passed in the publication audit. Fixture browser checks cover onboarding, landing, sigils, clipboard, maintenance/schedules, API failures and desktop/mobile layouts. Live Pi browser tests are opt-in through `HARBOR_BROWSER_URL`; skipped opt-in tests are not passing live proof. Current advisory checks are recorded in the readiness report, not inferred from a green unit suite.

The initial clean source archive was extracted independently, installed from its lockfile, rebuilt and checked again with race/vet/typecheck/unit tests. Its embedded production UI opened onboarding for an empty real registry and reached existing-folder setup, with no recorded browser errors. The isolated service stopped with exit 0. This checks a fresh source installation, not real ship boot or installer replacement. An initial port-0 smoke attempt exposed a configuration issue; the fixed-port retry passed. That initial publication preparation inspected the Pi read-only; the later disposable native trials are recorded below.

Native integration tests require explicit disposable paths/flags and some destructive interruption tests require the `harbor_chaos` tag. Never point them at a real identity. Raw reports, fixtures, reference screenshots and development notes are private local evidence and excluded from source/release bundles. Public claims above summarize the verified boundary without publishing personal paths, addresses or credentials.

## Release acceptance

An experimental source release can invite testing with explicit limitations. A stable release needs native acceptance on all four targets, full key/comet onboarding and real-identity migration, physical external-storage trials, representative resource profiles, accessibility checks and a verified macOS download/install flow with the documented manual approval. See [release readiness](release-readiness.md).

## Readiness follow-through — 2026-10-10

A fresh real comet booted through the production Mac ARM64 API and passed authenticated HTTP login. Its first early stop timed out and Harbor exited 1; that trial remains a preserved failure-window record. A separate rebuilt native trial passed stop/start, Pack/Roll/Chop, full export and archive restore. Replay correctly refused without a checkpoint belonging to the restored registration; after exporting that registration, replay/login and final graceful stop passed with service exit 0. This distinguishes a missing trial prerequisite from a product failure.

The actual restored ship also passed browser Dojo input `(add 5 6)`, the observed result `11`, host-correct Open ship, axe checks across three panels at desktop/mobile widths and visual screenshot inspection. Eight fixture screens passed automated accessibility/reflow/44px-control checks at 1280, 390 and 320 pixels. Human screen-reader/physical-touch acceptance remains separate.

The Pi virgin comet acquired an identity but failed opening initial LMDB metadata with `Out of memory`; the manager exited cleanly and preserved the pier/logs. This is default onboarding evidence, unlike older seeded fixtures. The controlled 64 GiB map passed bootstrap/login, graceful stop and a normal default-map restart. The earlier first-boot-only fix then passed the actual Harbor API: fresh comet, authenticated login, stop/start, Pack/Roll/Chop, full export and native archive restore. Replay exited 0, but its immediate restart reported `MDB_CORRUPTED`; copies of the post-replay pier booted with and without runtime locks. Original/checkpoint evidence is preserved. Failed post-utility restarts create durable holds; a helper-process regression verifies that preservation independently of the upstream failure.

Further isolated Vere 4.6 trials reproduced the startup fault in one of 16 default-map starts. A system-call trace and core analysis showed the fixed loom mapping replacing part of a live 500 GiB LMDB mapping and an adjacent allocator buffer. Independent read-only scans of the archived databases passed. Sixteen control starts with a 64 GiB map passed without overlap. This establishes the mechanism in a disposable fixture; the original live failure was not traced.

Harbor selects a map on every ship launch across all four supported macOS/Linux targets, with no architecture-specific trigger. It measures the largest metadata or epoch database by logical file size and chooses at least twice that size, rounded up to a power of two with a 64 GiB minimum. Separate epochs are not summed. The former 32 GiB database cutoff is removed; unsafe/uncheckable storage and sizes that cannot be represented with growth headroom are refused. Selection is repeated on each start, without resizing a running ship.

Focused regressions cover threshold boundaries, independent epochs, repeated-start growth, unsafe storage, cancellation and integer overflow. Isolated Harbor API trials on Mac ARM64 and Pi ARM64 each passed folder registration at 64 GiB, manual restarts selecting 128 GiB for a 40 GiB database and 256 GiB for a 100 GiB database, controller restart/autostart and post-Pack restart at 256 GiB. Authenticated login and actual process mappings without loom overlap were checked at all five starts on each host. The larger fixtures were valid database copies extended with sparse trailing space; they establish selection and native startup, not representative populated large-ship workload acceptance. A symlinked database import failed before any Vere process launched. Both test managers exited cleanly and their fixtures were removed; the Pi fixtures ran only in RAM. The lifecycle trials did not change the live Pi service or pier. Intel binaries cross-compiled; native Intel lifecycle acceptance remains open. Larger mappings can exceed a host's available address space, and the policy does not eliminate the upstream overlap bug. Those startup trials did not establish utility-process protection or renewed full comet replay acceptance.

Further lifecycle follow-through exposed a native Pi Pack utility crash after a verified disposable-comet import and two authenticated starts. Harbor retained the failed pier and recovery hold. The process reported an allocator fault; it was not traced, so its cause is not established by the earlier independent collision reproduction. Three frozen LMDB environments subsequently passed full read-only cursor scans, with all pier files and registry fingerprints unchanged. Structural readability does not establish successful recovery or valid application state.

The executor now remeasures adaptive map size under the pier guard for Pack/Meld/Roll/Chop and supplies the option immediately after the subcommand, before process recording or spawn. Actual Vere 4.6 parser probes accepted this placement, as did retained Vere 4.5 maintenance-parser probes; only 4.6 is in the current resolver catalog. Child-process regressions cover all four utilities, 64/128/256 GiB growth selection, unsafe-storage refusal and guard release. `play` and `prep` remain unchanged because Vere 4.6 rejects the map option in those parsers.

Matched offline Pack controls on independent copies of one verified stopped export both exited 0: one default 500 GiB map and one explicit 64 GiB map. Both inputs passed independent read-only scans; database hashes stayed unchanged and neither trace showed loom overlap. These two successful controls do not identify the cause of the earlier untraced Pack crash or prove address ownership. Failed piers and recovery holds remain preserved.

## Combined candidate acceptance — 2026-10-11

The combined alpha.3 Mac ARM64 candidate passed the original early-bootstrap shutdown window and resumed the preserved pier: shutdown took 75.8 seconds, both native process and controller exited 0, and a fresh-cookie authenticated session check passed after restart. Harbor now waits for private worker readiness and retains process ownership through graceful exit. See the [native bootstrap shutdown and replay restart record](validation/bootstrap-shutdown.md) for the reproduction, fixed trial and Pi collision controls.

A separate full Mac lifecycle used an independently verified export of a stopped fresh disposable comet. All 16 durable jobs completed: import, stop/start, Pack/Meld/Roll/Chop, backup/download/upload/restore, a checkpoint belonging to the restored registration, replay, another manual restart and final stops. Ten strict authenticated session checks passed; three archives passed content-hash verification. Both registrations ended Stopped with desired-running false and no recovery holds; the controller exited 0 with no native survivors. This is supplemental lifecycle acceptance (`freshOnboarding: false`), separate from the candidate's actual bootstrap-signal/resume trial. Pack/Meld/Roll arguments and all ship starts showed the selected 64 GiB map. Chop exited 0 but finished before direct argument capture; Mac mapping snapshots do not establish loom-overlap measurements.

The combined Pi ARM64 candidate passed the same supplemental lifecycle on owned SD folders, with all 16 durable jobs completed, all four maintenance commands and replay exiting 0, nine strict authenticated session checks, a successful extra manual restart and controller exit 0. The source export and the restored registration's own replay checkpoint were integrity-verified. Pack/Meld/Roll process mappings and all nine authenticated ship starts showed 64 GiB maps without overlap with the tested 2 GiB loom. Chop exited 0 and restarted successfully but was too brief for direct argument or mapping capture. This trial imported a verified stopped fresh-comet export (`freshOnboarding: false`); it does not repeat key onboarding, physical-drive acceptance or the historical failed Pack input.

| Combined alpha.3 candidate | Binary SHA-256 |
| --- | --- |
| macOS ARM64 | `f8bab77829c57a6ae156eee277020861f0dcc2204767faa5090907a0e9c9684e` |
| Linux ARM64 | `cfcd6b3ff3d6abd7cb00f7f6e47ab9e8b7781b7a3de9e7798e2b1aff1e4b44b4` |

The login contract requires a fresh cookie, the exact same-origin `/session.js` response, JavaScript content type and the expected parsed `window.ship` assignment. A transient landing-page 404 is not a failed login, and identity text in redirected login HTML is not successful authentication. Only initial session-script 404 responses are retried, for at most 120 seconds; nine real HTTP regressions verify those boundaries.

Retention regressions cover referenced uploads, pending imports, active jobs, recovery holds, minimum export retention with current integrity checks, corruption/changed-checkpoint refusal, canonical controller aliases, unknown files, changed/symlink targets, preview/apply and budget refusal. A separate CLI smoke check removed only expired disposable inputs after preview and preserved an unknown file. The live controller lock refused cleanup. No operator archive cleanup was performed.

No native Intel host, physical external drive or valid disposable boot key was available for this run. Native CI is prepared, but those hardware/key acceptance gates remain open. Harbor's current [macOS distribution](macos-distribution.md) uses manual approval without Developer ID signing or notarization. Browser-download allowance and public-release download were verified in the trials below; clean-Mac first installation remains open.

The [LMDB workaround maintenance note](lmdb-mapping-workaround.md) relates the adaptive policy to the [POSIX loom investigation](upstream/vere-loom-reservation-proposal.md), distinguishes ship-start coverage from utility processes, and defines the upstream ownership, native validation and default-capacity evidence required before retirement. Passing Harbor's selector tests alone does not establish those retirement gates.

## macOS installation menus — 2026-10-11

The `0.1.0-alpha.4-install.3` Mac app passed native menu checks with an empty disposable registry. **Close Window** left the app running without a visible window and preserved the same background runner PID. **Stop Harbor** unloaded the LaunchAgent, disabled startup and stopped HTTP; **Open Harbor** restarted the runner and restored the interface. Cancelling **Uninstall Harbor** preserved the app and the same runner PID. Confirming uninstall moved the test app to Trash, quit the interface and removed its LaunchAgent, copied executable and installation record. Configuration and a disposable data fixture remained unchanged.

These checks cover the installed candidate's interface and empty-registry service lifecycle. Shutdown with active ships retains its separate acceptance requirements. The later `0.1.0-alpha.4-install.4` disk image passed the browser-download trial below; this menu trial used `install.3`.

## macOS browser download and allowance — 2026-10-11

Safari downloaded the exact `0.1.0-alpha.4-install.4` disk image over a temporary loopback HTTP endpoint. Its SHA-256 matched the original candidate: `9c9ec3568d5630810322ada592747bbdd516bbd61c1362c8d823d67e2a07b529`. Safari applied a real `com.apple.quarantine` attribute. Finder opened the downloaded disk image and copied its app into `/Applications/Harbor.app`, preserving that quarantine. The first launch was blocked with **Harbor Not Opened**; the existing runner remained unchanged.

The documented `xattr -dr com.apple.quarantine "/Applications/Harbor.app"` command removed quarantine from all 12 affected app entries. Executable and signature contents were unchanged, and ad-hoc signature verification passed. Opening the same app through Finder then displayed Harbor's welcome screen. Its managed runner matched the downloaded binary and upgraded the existing `install.1` installation to `install.4`, retaining the exact configuration and empty registry. Quitting the interface left the same runner active; the downloaded volume was ejected successfully.

This verifies an actual browser download, the blocked first launch and the command on this Apple Silicon Mac. The Mac already had an empty Harbor installation, and the candidate was served locally because no public binary release existed at the time. First installation on a clean Mac and the **Open Anyway** alternative still require their own trials. Public-release download is recorded in the later trial below.

## macOS folder browsing — 2026-10-11

The installed `0.1.0-alpha.4-install.6` app opened the native macOS folder browser from existing-pier onboarding. The trial navigated outside the default pier root into an owned nested folder with spaces, selected it, and verified that its exact path appeared in the form. Reopening the browser and cancelling preserved that selection. Configuration and the empty ship registry were unchanged; no pier was imported or booted. The live runner and embedded frontend matched the fresh app build.

Four Browse regressions cover desktop selection/cancel, import error handling after an outside-root selection, and web-host parent/root navigation with explicit folder confirmation. The complete frontend suite passed 25 tests, with three opt-in real-ship tests skipped. Import and boot keep their existing path policy; browsing alone does not admit a new storage root.

## Browser and Linux directory navigation — 2026-10-11

The `0.1.0-alpha.4-install.8` build passed browser acceptance against real host filesystems in Chromium on an Apple Silicon Mac and Debian 13 ARM64 on the Raspberry Pi. The Linux trial extracted and ran the exact runner from the new `.deb`, using a separate empty registry. Both trials entered child folders, moved to parents outside the configured pier root, refreshed a listing after creating a new directory, selected a nested path containing spaces, navigated Home and filesystem root, entered paths directly, displayed hidden folders, recovered from unavailable paths, and cancelled without changing the chosen value. The Pi additionally rejected an OS-inaccessible directory with HTTP 403. No ship was imported or booted by either folder-browser test.

Screenshots and geometry checks cover 1280, 390 and 320 pixels. A screenshot review caught overlapping long breadcrumb labels; the final build fixes their sizing and keeps the current folder visible. Six controlled Browse regressions and all 27 frontend tests passed, including keyboard focus, Escape, reflow and accessibility checks. Four opt-in real-host/real-ship tests were skipped in the general suite; the folder-specific real-host test was run separately on both machines. Go race tests, vet, frontend type/unit tests and package-contract checks also passed.

The existing Pi runner was then upgraded with graceful service shutdown and an executable rollback copy. Its configuration, service definition, registered pier paths and desired-running settings were preserved. Fresh-cookie, exact `/session.js` checks verified JavaScript MIME and the registered ship identity before and after the update. Linux AMD64 packages are cross-built; this trial does not establish native Intel or privileged Debian package installation acceptance.

The `install.8` Mac app was also installed and launched. Its managed runner matched the bundled executable, the previous runner exited, and configuration and the empty ship registry were unchanged. In the installed native interface, Browse selected a real nested folder outside the pier root and cancellation preserved that path. A separate browser using the installed runner navigated to a parent and child outside the pier root, returned the selected path, and preserved it after navigating elsewhere and cancelling. These checks did not import or boot a ship.

## Latest Mac download and allowance — 2026-10-11

Safari freshly downloaded the exact `0.1.0-alpha.4-install.8` disk image from a temporary loopback HTTP endpoint. Its SHA-256 matched the immutable candidate: `d1340662ec3f5a3c9739c152cf47aed55153d170570af9f5eeb4e7340e834880`. Disk-image integrity verification passed, and Safari applied a real quarantine attribute. Finder opened that downloaded image and copied Harbor into Applications, retaining quarantine on all 12 app entries. Every bundle file matched the final candidate. The previous installed app was preserved as a rollback copy.

The first launch displayed **Harbor Not Opened** with Apple's unverified-download warning. The documented `xattr -dr com.apple.quarantine "/Applications/Harbor.app"` command exited 0 and removed all 12 quarantine attributes. Every bundle file remained unchanged and ad-hoc signature verification passed. Opening the downloaded app through Finder afterward displayed Harbor's welcome screen. The running app and copied runner were `install.8`; the runner matched the downloaded bundle. The same existing manager PID continued, and configuration and the empty ship registry were unchanged.

This is a fresh browser-download and command-allowance trial on an Apple Silicon Mac already running `install.8`. It does not establish first installation on a clean Mac, public binary-release delivery, an upgrade from a different version, or the **Open Anyway** alternative. The separate installed-build trial above covers the preceding runner replacement and Browse checks.

## Public Mac download and allowance — 2026-10-11

Safari downloaded the universal disk image directly from the official [`v0.1.0-main.2` GitHub prerelease](https://github.com/davetist/harbor/releases/tag/v0.1.0-main.2), built from commit `21f683e7b9fd649f25c3f6ce061cbeda61e8db7e`. The Mac builder, package checks and publication job passed. The download's SHA-256 matched the published `SHA256SUMS`: `18129ad433c84bd3b31d3432bfd377a755caa31895300f7aa46f5a135bd13aab`. Disk-image integrity, universal ARM64/x86_64 executable architectures and ad-hoc signature verification passed.

Finder copied the downloaded app into Applications and retained real Safari quarantine on all 12 app entries. The first launch displayed **Harbor Not Opened**; Done dismissed the warning. Running the documented `xattr -dr com.apple.quarantine "/Applications/Harbor.app"` command removed all 12 attributes without changing any bundle file. Signature verification still passed, and opening the same downloaded app afterward displayed Harbor's welcome screen.

The app replaced the existing `install.8` runner with `0.1.0-main.2`; the previous runner exited and the new running executable matched the downloaded bundle. Configuration and the empty ship registry were unchanged. Served frontend assets matched the tested Browse build, and native Browse opened the folder navigator and cancelled without changing the form. Harbor was left open on its welcome screen. The previous app was kept as a rollback copy, and the temporary download volume was ejected.

This verifies a public browser download, command allowance, managed version replacement and interface smoke check on this existing Apple Silicon Mac. It does not establish first installation on a clean Mac, native Intel execution, the **Open Anyway** alternative or real-ship lifecycle acceptance.

## Current public Mac download and allowance — 2026-10-11

The subsequent packaging update published [`v0.1.0-main.3`](https://github.com/davetist/harbor/releases/tag/v0.1.0-main.3) from commit `751441fdf6c1519311f86cab64069fb1e3cb8444`, with unchanged application source. This exact release was freshly downloaded through Safari and tested independently. Its SHA-256 matched the published manifest: `db25133cac4937b0663c56bfa33e5abe3cd4e68f503c13334556ffc1b1279eea`. Disk-image integrity, universal executable architectures and ad-hoc signature verification passed.

Finder copied the downloaded app into Applications with real Safari quarantine on all 12 bundle entries. The first launch displayed **Harbor Not Opened**, dismissed with Done. The exact documented `xattr -dr com.apple.quarantine "/Applications/Harbor.app"` command exited 0, removed all 12 attributes and preserved every bundle file and its integrity signature. Opening that same downloaded app afterward displayed Harbor's welcome screen.

The running app and managed runner were `0.1.0-main.3`; the copied runner matched the downloaded bundle and the previous `main.2` runner exited. Configuration and the empty ship registry were preserved. Existing-pier navigation and native Browse opened correctly; cancelling the picker preserved the empty form. The tested app remained open on its welcome screen, the previous app was retained for rollback, and the download volume was ejected. The clean-Mac, native Intel, **Open Anyway** and real-ship limitations of the preceding trial still apply.

## Linux installer formats — 2026-10-12

The local `0.1.0-main.12-unified.3` candidate builds both `.deb` and `.run` for Linux ARM64/AMD64 with the same CLI, application-menu launcher and per-user background runner. The full release builder also produced the existing universal Mac disk image and allowlisted archives. All output checksums passed. The ARM64 standalone payload matched its outer manifest, runner and final installer source after transfer to the Raspberry Pi. These candidates have not been published.

On Debian 13 ARM64, the final runner passed the disposable native service trial: installation, idempotent opening, replacement of the public `main.12` binary, graceful stop/start and removal, with configuration and a data marker retained. The test used a unique systemd service and empty temporary registry. Native Linux CLI tests passed for the common boot-policy default, exact package-source filtering, preserved lingering during package upgrades, and accounts with UID 501 and 1000. The native Mac service trial passed on Apple Silicon; it used the earlier `unified.1` runner before the subsequent Linux-only installer corrections.

The final working tree passed `go test -race -p 1 ./...`, native Mac and cross-platform Linux CLI vet, frontend type/unit checks, nine acceptance-script regressions, seven package-contract tests, shell syntax and diff checks. Package regressions verify Debian parent-directory entries, standalone CLI/launcher installation and removal, modified-file preservation, unsafe directory refusal, Debian/standalone conflict handling, partial Debian installation states, normalized prefix variants, architecture refusal and source-archive inputs. Portable system-file tests simulate privileged metadata and locking; they do not establish the root boundary on a real Linux host.

The installed public `main.12` Debian package on the Pi supplied the normal `harbor` command. Its desktop entry was launched through GIO in the real graphical session, and Harbor's empty-registry interface was inspected in Chromium through VNC. The initial public package needed its missing parent directories prepared before dpkg could install; the new archive regression and builder fix address that fresh-install defect.

Privileged fresh Debian unpack and standalone system-file install/removal of the final candidate passed on the Pi in a new root-owned fixture. The checks exercised real root ownership, repeat installation, manifest verification, unsafe-ancestor refusal and format-conflict guards. The existing service record, copied runner, unit and PID stayed unchanged, and the fixture was removed. The final Debian package then upgraded the actual Pi installation successfully. Its running executable matched the package, the live API reported the expected version and SSD pier root, and configuration and the empty registry were retained. The final desktop entry was launched through GIO in the graphical session.

An actual format-switch trial on the empty Pi installation also passed: Debian package removal, ordinary-user execution of the final `.run`, standalone removal through its installed helper, then Debian package reinstallation and ordinary-user service setup. The standalone installer supplied its system CLI, desktop entry and running SSD-backed service. Both removal paths retained the original configuration, empty registry and a private SSD data marker; standalone removal also retained existing user lingering. The marker was removed after the Debian installation was restored, and SQLite integrity remained valid.

The physical SSD became unavailable after an xHCI controller failure and ext4 shutdown. Harbor was stopped before normally detaching the failed mount and resetting the controller. The expected drive identity and filesystem UUID reappeared; an unmounted read-only filesystem scan passed, and journal recovery completed. Further UAS read timeouts during the scan required a separate adapter comparison. With the kernel's adapter-specific `IGNORE_UAS` quirk, the same filesystem scan and a 1 GiB write/uncached-read hash check passed with no new kernel errors. Fsync, rename, hard links and writable memory mapping also passed, the temporary files were removed, and the USB 3 link was retained. This compatibility setting was backed up and saved for future boots. Harbor was restarted with the original UUID-pinned mount and private SSD folders. These are empty-registry filesystem checks, separate from real-ship migration and long-running storage reliability.

An authorized Pi reboot exposed an OS mount-order failure: the SSD remained an autofs mount while Harbor's required-volume guard refused startup. Accessing the configured pier directory triggered the ext4 mount, after which the background service recovered with the expected version and empty registry. The adapter compatibility driver was present after reboot. The temporary boot observer timed out without producing a completed acceptance record. VNC subsequently returned and displayed Harbor's welcome screen, then both VNC and SSH became unavailable. The operator reported that boot succeeded after powering off and disconnecting the SSD. SSH, the desktop, VNC and Tailscale services were confirmed running on the subsequent boot, with no current kernel storage or undervoltage errors and no current throttling flags. Harbor refused startup without its required volume, and its retries were paused while retaining the enabled boot preference. Previous-boot kernel logs were unavailable, so the SSD-connected boot failure's cause remains unconfirmed.

The operator executed the copied privileged mount repair while the SSD was disconnected. It backed up `/etc/fstab`, retained the exact UUID and mount point, removed the on-demand automount, and ordered the direct mount before the service user's manager. The generated dependency and absent automount unit were verified. The expired boot-test files were removed, configuration and the enabled boot preference were retained, and the unmounted directory remained empty with mode `0555`.

After the operator connected the SSD to a USB 2 port, a subsequent boot was verified against the repair timestamp. The matching adapter used `usb-storage` at 480 Mb/s; the exact UUID mounted at 7.34 seconds and Harbor started at 14.01 seconds with zero retries. The startup filesystem-check service executed successfully and reported a clean filesystem. The live runner matched the final package, the registry stayed empty, and SQLite integrity passed. A separate ordinary-user test wrote and synced 1 GiB of unique random blocks, verified an uncached SHA-256 read-back, and exercised rename, hard links and writable memory mapping. It passed in 71.8 seconds, removed its disposable files, and recorded no kernel storage errors or power/throttling flags. This validates the USB 2 setup; the USB 3 failure remains unresolved. Desktop autologin preceded Harbor in this boot, so that observation alone did not establish startup without login.

A separately authorized reboot passed the revised one-use startup observer. Normal systemd ordering held console and desktop login until the observer exited; SSH was not opened until its checks and cleanup finished. Harbor served its API with no user login session and LightDM inactive. The observer verified the exact SSD UUID, USB 2 compatibility driver/link, running package hash, preserved configuration, enabled service and an ordinary-user write/fsync/read test, with no kernel storage errors. The SSD mounted at 7.31 seconds, Harbor started at 13.97 seconds with zero retries, and LightDM started at 16.67 seconds. The result belonged to the new current boot and reported no cleanup errors. Both old and revised boot fixtures were absent afterward, the normal graphical target and console login were restored, and VNC, desktop and Tailscale services were active. A fresh live inspection confirmed the same running binary, private SSD folder permissions, unchanged configuration, valid SQLite and empty registry. Native Intel execution and graphical removal of the final package remain separate acceptance gates.

After private VNC authentication, the installed desktop entry opened Harbor's welcome screen in the Pi's Chromium browser. Actual keyboard interaction reached the existing-pier form, whose default folder was the configured SSD pier root. Opening the interface retained the same background runner PID and unchanged configuration. The operator then closed Chromium; VNC showed the desktop, all browser processes exited, and Harbor's API and service remained running with the same PID and zero service retries. The temporary desktop-launch unit was automatically collected. The final SSD mount was still read/write with no kernel storage errors, configuration was unchanged, and SQLite integrity passed. No ship was imported or started during this desktop check.
