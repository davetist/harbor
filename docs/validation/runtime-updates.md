# One-click runtime updates

Evidence recorded on 2026-10-12 using a rebuilt macOS ARM64 runner and two disposable fake ships. This trial used an isolated registry and official Vere 4.5 and 4.6 binaries; it did not update an existing service or operator ship.

## Behavior

**Update runtime** resolves and verifies Harbor's catalog runtime while the ship can remain running. When the executable differs, Harbor stops an owned running ship, runs the old runtime's `prep`, saves the new runtime selection, and restores the previous running state. A stopped ship stays stopped. A matching runtime digest completes without preparation or restart.

There is no prior-export requirement and no Harbor-generated full-pier archive. Vere's upgrade preparation makes the native checkpoint current, and its version-transition handling preserves epoch snapshots. The previous executable and event history remain available. Failed or uncertain preparation and failed restarts retain durable recovery holds; Harbor does not automatically roll back network history. Full replay retains its separate verified-export requirement.

This policy is based on the supported runtime's [preparation procedure](https://docs.urbit.org/user-manual/running/vere#prep) and [epoch snapshot handling](https://github.com/urbit/vere/blob/vere-v4.6/pkg/vere/disk.c#L1060-L1093). Future catalog transitions still require native migration validation.

## Native browser trial

The running fake `~zod` passed a fresh authenticated login on Vere 4.5 before the update. Its sibling stopped fixture was fake `~bus`. Neither registration had an export. Each update was initiated by clicking **Update runtime** in the production browser interface.

| Check | Result |
| --- | --- |
| Running 4.5 → 4.6 update | Completed with preparation exit 0; running state and desired-running intent restored. |
| New-runtime login | Fresh cookie and exact `/session.js` identity verification passed. |
| Stopped 4.5 → 4.6 update | Completed with preparation exit 0; remained stopped with desired-running false. |
| Export prerequisite and automatic archive | Zero export records before and after both updates; no full-pier archive created. |
| Stopped ship's later explicit Start | Booted the selected Vere 4.6 runtime and passed a fresh authenticated login. |
| Previous runtime | Original Vere 4.5 executable retained. |
| Final manager shutdown | Graceful exit 0. |

The stopped-state assertion was recorded before the separate explicit Start check. The rebuilt runner SHA-256 was `87ad8efc7db190aee546d7e337e76572e7238506f92adec3069e03818f867fe4`.

The bootstrap harness initially compared an identity response without trimming its trailing newline. That harness was stopped gracefully, its evidence preserved, and the corrected readiness check resumed the stopped fake fixture. This was a harness defect before the runtime-update trial, not an update failure.

## Automated checks and limits

Focused regressions first failed on the old export gate, then passed after its removal. They exercise the real executor, cache digest/architecture checks, store, owned helper subprocesses, graceful stop and readiness handling. They cover running/stopped updates without exports, no additional archive, retained old executable, failed preparation, failed restart, and an already-current runtime leaving the live process undisturbed. Existing download-failure and replay-export regressions also passed.

The ignored local maintenance probe's obsolete `pier.VerifyRegistered` argument was corrected, removing the initial whole-tree compilation blocker. The production build, full `go test -race -count=1 ./...`, full `go vet ./...`, frontend typecheck/unit tests, module/notices checks, installation-package tests and nine native-login harness regressions passed. The initial browser run passed 49 tests with four native opt-in checks skipped. A subsequent [native disposable-fixture trial](native-browser-checks.md) enabled those checks on macOS ARM64 and passed all 53 browser tests with zero skips.

This native evidence covers macOS ARM64 and Vere 4.5 → 4.6. Failure holds were checked with helper processes. It does not establish native acceptance on Linux or Intel, other runtime transitions, physical storage failures, power loss, or representative large active ships.
