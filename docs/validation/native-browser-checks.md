# Native browser checks

Evidence recorded on **12 October 2026** on native macOS ARM64, using a fresh production build and official Vere 4.6. Two new Harbor services used loopback listeners and separate temporary registries. No existing service or operator ship was used.

## Fixtures

The filesystem fixture had an empty registry, an ownership marker, nested folders, names containing spaces and an ampersand, and hidden folders. Its configured pier root was checked before and after the browser trial.

The ship fixture was a freshly booted fake `~zod` created with Vere's `-F` option, which disables networking, and the official `urbit-v4.6.pill`. It stopped gracefully before registration through Harbor's existing-pier production API with its original 2 GiB loom. Harbor started the registered ship on Vere 4.6. A separate fresh authenticated login verified the issued cookie and exact `/session.js` identity before the browser tests.

## Results

The full browser suite ran with `HARBOR_BROWSER_URL`, `HARBOR_ACCEPTANCE_URL`, `HARBOR_FOLDER_BROWSER_URL` and `HARBOR_FOLDER_BROWSER_ROOT` pointing to these disposable fixtures. `npm --prefix web run test:e2e -- --workers=1 --reporter=json` completed with **53 passed, zero skipped, zero failures and zero flaky tests**.

| Previously skipped check | Result |
| --- | --- |
| Native host folder browsing | Passed traversal, path entry, fresh listings, hidden folders, selection/cancel, and layouts at 1280, 390 and 320 pixels. The registry remained empty. |
| Native ship accessibility | Passed the configured WCAG 2 A/AA, 2.1 A/AA and 2.2 AA axe checks for Overview, Maintenance and Terminal at 1280 and 390 pixels; Dojo returned the multiplication result and Open reached the ship's login page. |
| Production navigation, terminal and Open | Passed against the connected native service and actual Vere terminal, including the ship sigil, command submission, maintenance controls and ship login page. |
| Production Roll/Chop schedule edits | Passed shared-cadence changes while preserving each schedule's enablement. The original cadence and enablement were verified after the suite. |

Bootstrap, both managers and Harbor's recorded ship process exited with code 0. The ship was explicitly stopped, all jobs completed, and no surviving fixture process remained. The disposable pier was removed after those checks; private reports, logs and screenshots were retained locally.

| Artifact | SHA-256 |
| --- | --- |
| Harbor runner | `6935363d365e3d39db0a9ede272dc80bd9c4c83ccd2e42cc4cc36ad6b4b8e93b` |
| Vere 4.6 executable | `1794e75c15162298d43c1e5e2e4f643e989caf04360562af170050a0d919f1ce` |
| Bootstrap pill | `f8b08c53dd6d0aa9c4f7654f0a7bcd11e94089222888a3c99c0a10eedcbb377b` |

This trial establishes these browser workflows on macOS ARM64 with a disposable fake ship. It does not establish native Intel/Linux acceptance, real-identity migration or physical-failure recovery.
