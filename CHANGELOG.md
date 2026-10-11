# Changelog

## Unreleased

Initial experimental Harbor implementation and publication preparation. No public release has been published.

- Universal Mac app/disk image with automatic user-service setup, native file pickers/downloads, persistent background operation and explicit stop/uninstall controls.
- Debian/Ubuntu desktop packages with application-menu launch/stop/removal, plus offline one-command ARM64/amd64 headless installers.
- Managed installation records, data-preserving upgrades/removal, previous-runner rollback, logged-out-user package handling and service-PID readiness checks.
- macOS distribution without Developer ID signing or notarization, with manual first-launch approval and a Harbor-only quarantine command included in the disk image and installation guide.
- Native installation lifecycle acceptance on Apple Silicon and Raspberry Pi, plus a real Safari download, blocked macOS launch, documented quarantine command and successful native launch/upgrade. Clean-Mac first installation, public-release delivery and privileged desktop-package/boot trials remain separate gates.

- Native Go service with an embedded React browser interface for macOS/Linux, x86_64/ARM64.
- Key/comet onboarding, stopped-folder registration and integrity-checked full-pier archives.
- Persistent ships/jobs, process ownership, graceful shutdown and interrupted-work recovery holds.
- Upstream Urbit sigils, access codes through private loopback HTTP, and PTY-backed Dojo.
- Pack/Meld/Roll/Chop, exact paired scheduling and measured resource admission profiles.
- Runtime update/replay, migration export semantics and startup volume/UUID guards.
- Default Harbor port 18789, LAN/external-tailnet access and browser-host ship URLs.
- MIT source license, public documentation, build/check workflows and source/binary packaging.
- Development dependency advisory remediation. Remaining blockers are listed in release readiness.
- Require Go 1.27.2 or newer for source builds and hosted workflows, incorporating the patched standard library used by the local validation.
- Fixed repeated archive symlink-chain validation with a scan-local resolved-path cache and post-read cancellation checks; preserves cycle, missing-target and integrity rejection.

### Readiness follow-through

- Added offline upload/export retention preview/apply, disk accounting, cleanup budgets and protection for pending jobs/recovery/checkpoints. Startup key cleanup preserves referenced inputs.
- Corrected port-zero Host validation and exposed manager/pier storage locations distinctly.
- Improved keyboard focus, accessible tabs/upload selection, contrast, touch targets and narrow layouts; added fixture and opt-in native accessibility regressions.
- Added disposable native onboarding/lifecycle tooling, a manual four-platform acceptance workflow and a universal Developer ID signing/notarization installer path (superseded by the current unsigned app distribution). Hardware/key acceptance remains explicit in release readiness.

- Added adaptive LMDB mappings on every macOS/Linux ship launch on x86_64 and ARM64, including first boot, imports, manual/automatic starts and post-utility restarts. Mapping size is at least twice the largest database's logical file size, rounded up to a power of two with a 64 GiB minimum and recalculated on each start. There is no 32 GiB database cutoff or live resizing. Native Mac ARM64 and Pi ARM64 trials verified 64/128/256 GiB mappings and authenticated login, automatic startup, post-Pack restart and unsafe-storage refusal on isolated fixtures. Larger fixtures used valid databases with sparse trailing space; populated large-ship acceptance and native Intel execution remain open.
- Extended adaptive mapping to Pack/Meld/Roll/Chop utility processes, remeasuring under the pier guard before spawn. Child-process regressions cover all four utilities, database growth, unsafe-storage refusal and unchanged replay/update-preparation arguments. Vere 4.6's `play` and `prep` parsers do not support the map option.
- Require a private worker identity reply before reporting startup ready. During first boot, graceful stop waits until Vere can handle the signal; Harbor keeps the controller, PTY, database and process ownership until native exit instead of closing them at a fixed shutdown deadline.
- Corrected native login acceptance for transient landing-page absence: require a fresh session cookie and an exact same-origin JavaScript session response assigning the expected ship identity. Login redirects, HTML and authentication refusal remain failures; only initial session-script 404 responses receive bounded retries. Nine HTTP contract regressions are included in source packages and routine CI.
- Documented the mapping workaround's relationship to the POSIX loom collision investigation, unsupported utility paths and retirement criteria. Removing collision mitigation requires verified upstream address ownership; removing explicit sizing also requires compatible defaults and native capacity checks.
- Refreshed experimental packaging for `0.1.0-alpha.3`. Source packages include the adaptive mapping implementation and the exact sanitized public trace/core evidence and PTY reproduction helper. The public evidence manifest matches the approved issue attachment; sigil tests use a sample identity. Earlier candidates remain historical artifacts.
- Canonicalized controller/pier ownership locks across folder aliases and retained currently integrity-checked checkpoints during cleanup.
- Persisted recovery holds after failed post-utility restarts. The earlier Pi replay failure was reproduced as a traced LMDB/loom collision; failed inputs and holds remain preserved.
- Verified combined alpha.3 ARM64 candidates through supplemental stopped-comet import, Pack/Meld/Roll/Chop, verified export/restore/checkpoint replay, authenticated restart and a further manual restart. Mac early-bootstrap signal/shutdown/resume also passed. These trials do not certify native Intel ships, real identities, physical-drive failures or broader recovery windows.
