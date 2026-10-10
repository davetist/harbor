# Native bootstrap shutdown and replay restart

Validated on the actual Mac ARM64 and Raspberry Pi ARM64, using disposable piers. Failed inputs and private traces are retained separately from the public source tree.

## Mac bootstrap shutdown

The baseline reproduced a fresh-comet onboarding job completing without a confirmed private identity. The acceptance attempt then failed, and controller shutdown hit the fixed 60-second stop deadline, exiting with status 1 while its durable process record remained live.

Vere 4.6 can answer public HTTP before its worker handles a private query. During earlier first-boot processing, its graceful-exit handler also has no registered pier to stop. One early TERM followed by a short deadline therefore does not establish a clean native exit. Closing Harbor's controller, PTY and database at that deadline loses tracking before the native shutdown is confirmed.

Harbor now requires a valid private worker identity before marking a ship Running. If shutdown interrupts a new boot, it waits for that worker response before signaling the still-owned process, rechecks the process identity after waiting, and retains the controller/PTY/database until graceful exit is recorded. Caller cancellation leaves the child and pier guard owned; no force-kill or recovery-hold bypass is introduced. The HTTP server's final shutdown retains its separate deadline.

The native fixed trial sent controller SIGTERM while the comet job was Executing and the ship was Starting, after observing Vere's bootstrap-start marker. Shutdown took **70.25 seconds**; both native child and controller exited **0**. The preserved pier was Stopped without a recovery hold. A new controller then started that pier, confirmed its private identity, accepted its login and stopped it, again exiting 0. This exceeds both the old 60-second stop window and 65-second controller window.

The combined `0.1.0-alpha.3` Mac ARM64 candidate repeated that exact bootstrap-signal/preserved-pier sequence. Its binary SHA-256 is `f8bab77829c57a6ae156eee277020861f0dcc2204767faa5090907a0e9c9684e`. Shutdown took **75.8 seconds**, with native process records Exited/0 and controller exit 0. The resumed pier passed a fresh-cookie authenticated check of the exact `/session.js` URL, JavaScript content type and expected `window.ship` assignment, then stopped and closed its controller with exit 0. Login redirects or identity text embedded in HTML do not count as that proof.

Supervisor regressions cover premature public-HTTP readiness, shutdown after canceled launch, clean process/guard release, and canceled-stop ownership preservation. Existing export/process helpers now supply the same private identity response required by production readiness. The complete Go race suite and vet pass.

## Pi post-replay restart

An isolated copy of the previously failed comet's post-replay state reproduced the original failure on the **third default-map startup**: `MDB_CORRUPTED` followed by the Vere assertion. The failed copy was not retried. Its syscall trace shows the worker's 500 GiB LMDB mapping beginning at `0x27ff3a000`, followed by a fixed 2 GiB loom mapping over `0x200000000`–`0x280000000`. The overlap is **811,008 bytes**.

Eight matched native startup controls used the same frozen input and archived-lock policy with a 64 GiB map. All eight exited 0, without corruption or assertion; their actual LMDB/loom mappings had zero overlap. These are fresh device observations, not an extrapolated failure probability.

The adaptive [launch mapping policy](../lmdb-mapping-workaround.md) applies at the post-replay supervisor restart, remeasuring the database with growth headroom. Replay's own `play` parser does not accept that option, so its arguments remain unchanged. The 64 GiB control is evidence for the reproduced fixture; it does not replace the upstream address-ownership fix or the mapping policy's documented larger-database limits.

The combined `0.1.0-alpha.3` Linux ARM64 candidate, SHA-256 `cfcd6b3ff3d6abd7cb00f7f6e47ab9e8b7781b7a3de9e7798e2b1aff1e4b44b4`, then passed the production API workflow on the Pi: import of a verified final stopped export of a newly booted disposable comet; authenticated login and manual restart; Pack/Meld/Roll/Chop; export/restore; a checkpoint belonging to the destination; full replay with utility exit 0; automatic restart with authenticated login; and another manual stop/start/login. All **16 durable jobs** completed, all recorded native processes exited 0, and the controller exited 0. Both piers ended Stopped without recovery holds or surviving writers. **Nine authenticated session checks** required fresh cookies, the exact `/session.js` URL, JavaScript content and the expected ship assignment.

Actual parent/worker starts selected 64 GiB with no database/loom overlap. Utility mappings were directly observed for Pack/Meld/Roll; Chop's short process window was not captured, so its narrower evidence is source/parser/helper flag validation plus native exit 0 and authenticated restart. This supplemental cycle starts from the current stopped comet export; earlier fresh onboarding is separate evidence. The [validation boundary](../validation.md) retains that distinction and the wider acceptance limits.
