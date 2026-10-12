# Harbor version and update checks

Evidence recorded on **12 October 2026** on native macOS ARM64.

## Behavior

The bottom-right version label is the update control. Clicking the installed version displays **Checking for updates…** until the request completes. An available version replaces that label in light yellow; clicking it opens Harbor's public GitHub releases in a new browser tab. Its tooltip retains the installed version. There is no separate update button or banner.

The service checks published installer releases through GitHub's public API without credentials or ship information. It filters drafts, malformed versions, unpublished releases, unfinished uploads and releases lacking a matching host installer or `SHA256SUMS`. Versions compare numerically, including prerelease identifiers. Stable builds exclude prereleases. Development builds and unrecognized version strings do not claim to be outdated.

Background checks run when the interface opens, on focus and hourly, sharing a six-hour service cache. Manual checks refresh immediately. Failed background checks remain quiet; failed rechecks retain any previously discovered update. Checking does not install software, submit ship jobs or change running intent.

## Automated checks

- Production UI/runner build, frontend typecheck and unit tests passed.
- Full `go test -race -count=1 -p 1 ./...` and `go vet ./...` passed. The checker package was also race-tested after adding both Linux installer-architecture cases.
- Module download, notices generation, four installation-package regressions, nine native-login harness regressions and dependency audit passed; audit reported zero vulnerabilities.
- All four supported macOS/Linux ARM64/x86_64 runner targets built.
- Full browser suite passed **66 tests, zero skipped, zero failed and zero flaky** with all five native checks enabled.

The focused browser regressions cover the version as a button, checking progress, manual discovery/current-version results, the light-yellow releases link, offline/cached results, development builds and background errors. Desktop, 390-pixel and 320-pixel layouts passed overflow and the configured WCAG 2/2.1/2.2 A/AA axe checks.

An initial parallel Go run saw port 8081 become unavailable between a port-selection test's probe and reservation; a separate Vere listener was present. The full serialized run passed without altering that assertion. A build/test overlap also exposed generated UI assets being replaced during compilation; rebuilding first and then running the full suite resolved it.

## Native production trial

Three disposable Harbor services used separate temporary registries and loopback listeners: an empty filesystem fixture, an older-version fixture managing a freshly booted fake `~zod` on official Vere 4.6, and an empty current-version fixture. The locally compiled runners deliberately advertised `0.1.0-main.1` and the live release `0.1.0-main.7` to exercise both UI states; these were validation builds, not downloaded historical Harbor binaries.

The older fixture discovered the live installable release through GitHub, showed its version in light yellow, and opened the actual GitHub releases page. The matching universal Mac installer returned HTTP 200 to HEAD. The current fixture's version button performed a real refresh and reported Harbor up to date. The ship's state and desired-running intent were unchanged by update checks.

The existing native folder-browser, accessibility/Dojo/Open, production navigation and Roll/Chop schedule-edit checks also passed. A fresh authenticated ship login passed before the suite. All jobs completed; bootstrap, the ship and all three managers exited with code 0. No fixture process remained. The stopped disposable pier was removed after ownership/process checks; private logs, reports and screenshots were retained locally.

| Validation artifact | SHA-256 |
| --- | --- |
| Older-version Harbor runner | `4a4c88d6a23c809f76d85a09f9f793b25442fe48cdbf66160dbfd639958ae58f` |
| Current-version Harbor runner | `d7f2ff4612c6dea279f68925d8ed8b088731391ae6db3b42b5924fc85c909a48` |
| Vere 4.6 executable | `1794e75c15162298d43c1e5e2e4f643e989caf04360562af170050a0d919f1ce` |
| Bootstrap pill | `f8b08c53dd6d0aa9c4f7654f0a7bcd11e94089222888a3c99c0a10eedcbb377b` |

Native browser evidence covers macOS ARM64. The change reads release metadata and opens a website; it does not change installer replacement, OS service registration/removal or ship runtime upgrade behavior. Native installation/service lifecycle and runtime-transition trials are therefore outside this change's acceptance scope.
