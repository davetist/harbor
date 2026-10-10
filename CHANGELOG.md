# Changelog

## Unreleased

Initial experimental Harbor implementation and publication preparation. No public release has been published.

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
- Added disposable native onboarding/lifecycle tooling, a manual four-platform acceptance workflow and a universal Developer ID signing/notarization installer path. Hardware/key/signing acceptance remains explicit in release readiness.

- Added adaptive LMDB mappings on every macOS/Linux ship launch on x86_64 and ARM64, including first boot, imports, manual/automatic starts and post-utility restarts. Mapping size is at least twice the largest database's logical file size, rounded up to a power of two with a 64 GiB minimum and recalculated on each start. There is no 32 GiB database cutoff or live resizing. Native Mac ARM64 and Pi ARM64 trials verified 64/128/256 GiB mappings and authenticated login, automatic startup, post-Pack restart and unsafe-storage refusal on isolated fixtures. Larger fixtures used valid databases with sparse trailing space; populated large-ship acceptance and native Intel execution remain open.
- Documented the mapping workaround's relationship to the POSIX loom collision investigation, uncovered utility paths and retirement criteria. Removing collision mitigation requires verified upstream address ownership; removing explicit sizing also requires compatible defaults and native capacity checks.
- Refreshed experimental packaging for `0.1.0-alpha.3`. Source packages include the adaptive mapping implementation and the exact sanitized public trace/core evidence and PTY reproduction helper. The public evidence manifest matches the approved issue attachment; sigil tests use a sample identity. Earlier candidates remain historical artifacts.
- Canonicalized controller/pier ownership locks across folder aliases and retained currently integrity-checked checkpoints during cleanup.
- Persisted recovery holds after failed post-utility restarts; the new Pi replay failure remains an explicit acceptance gate.
