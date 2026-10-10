# LMDB mapping workaround and retirement criteria

Status recorded on **10 October 2026**: Harbor applies adaptive mapping on every ship launch. The validated runtime is **Vere 4.6**. No fixed upstream release or retirement version has been recorded for this workaround. The policy does not automatically disable itself after a version update.

Code links in this document resolve in the matching source archive. Binary packages include the documentation and public investigation evidence; obtain the companion source package to inspect the implementation and tests.

## Relationship to the investigation

The [POSIX loom investigation and proposal](upstream/vere-loom-reservation-proposal.md) reproduced a Vere 4.6 worker mapping its loom with `MAP_FIXED` over part of a live LMDB mapping and an adjacent allocator buffer. This produced `MDB_CORRUPTED` and an allocator fault even though independent reads of the frozen input databases passed. The [sanitized investigation evidence](upstream/vere-loom-collision-evidence.json) records the trace and proof limits.

The mechanism is unsafe address replacement in Vere. It does not establish that every `MDB_CORRUPTED` report has this cause or that the reproduced failure damaged the database on disk. The original live failure was not traced.

Reducing the LMDB mapping avoided overlap in the tested control series. Harbor uses that mitigation while leaving the upstream fix explicit: Vere must acquire and own the required native and migration loom ranges before other allocations, and must never replace unrelated mappings. `--lmdb-map-size` controls database capacity; it does not enforce this ownership invariant.

The source concern covers the shared POSIX path. Harbor therefore applies one policy to macOS and Linux on ARM64 and x86_64, rather than using Linux ARM64 as a fault detector. The collision was reproduced on Linux ARM64; equivalent runtime failure on the other targets has not been established.

## Selection and lifecycle coverage

[LMDBMapArgs](../internal/runtime/lmdb.go) reads logical file lengths of `.urb/log/data.mdb` and `data.mdb` in each immediate log subdirectory. Separate epoch environments are not summed; the largest file determines the shared launch option. Snapshot sizes, total pier size, allocated disk blocks and historical LMDB map-size metadata do not determine this choice.

```text
mapping = nextPowerOfTwo(max(64 GiB, 2 × largest_database_logical_bytes))
```

The implementation uses checked integer doubling, without multiplying a potentially overflowing database size. New, absent pier destinations use the 64 GiB minimum. The previous 32 GiB per-database admission cutoff is removed.

| Largest database file | Selected virtual map |
| --- | --- |
| New pier or at most 32 GiB | 64 GiB |
| More than 32 GiB, at most 64 GiB | 128 GiB |
| More than 64 GiB, at most 128 GiB | 256 GiB |
| More than 128 GiB, at most 256 GiB | 512 GiB, subject to host address-space limits |

The 64 GiB floor comes from the successful control size. Twice the database length and power-of-two rounding are Harbor's growth-headroom policy, not a requirement of the upstream proposal or a proven collision-free size. At startup, the map is at least twice the measured file length.

[Supervisor.launchShip](../internal/supervisor/supervisor.go) selects the map after path, writer and Harbor guard checks, before spawning Vere. It prepends `--lmdb-map-size <bytes>` to the launch arguments. The native trials inspected both the parent and exec-created worker; forwarding to the worker is necessary because it opens the database and maps the loom.

| Path | Adaptive selection applies? |
| --- | --- |
| Fresh key/comet boot | Yes, initially 64 GiB. |
| Existing-folder registration and archive restore startup | Yes, from restored database sizes. |
| Manual Start and controller automatic startup | Yes, remeasured on each launch. |
| Ship restart after maintenance, replay or runtime update | Yes, when the executor restarts the ship through the supervisor. |
| Pack/Meld/Roll/Chop utility process itself | No; utility arguments are unchanged. |
| Replay (`play`) and update preparation (`prep`) process itself | No; Vere 4.6's argument parsers do not accept this flag. |

See [BuildArgs](../internal/runtime/args.go) and the [utility executor](../internal/jobs/executor.go). Successful post-utility startup does not establish protection of every utility process. Failed post-utility restarts retain recovery holds and checkpoints.

## Limits and maintenance implications

- This is virtual address space, not a request to allocate that many bytes of physical RAM or disk. Pages actually accessed still consume resources. File length is measured conservatively, including sparse trailing space.
- There is no live resize. Growth during a run can exhaust the map. A later launch remeasures the files; the policy is not a guarantee of unlimited uptime or automatic map-full recovery.
- Larger databases select larger maps, potentially approaching or exceeding the defaults implicated in the investigation. A smaller map does not guarantee disjoint addresses, and another allocation can still occupy a loom range. OS/architecture, ASLR, kernel page layout, loom size and runtime changes require renewed validation.
- A contiguous map may not fit the host, especially at 512 GiB or above on constrained address spaces. Harbor does not silently shrink below the selected headroom or retry with the upstream default. A correct upstream reservation fix would prevent destructive overlap but would not create more address space.
- The scan rejects symlinked/nonregular databases, unsafe or unreadable log storage, more than 10,000 immediate log entries and sizes whose mapping cannot be represented in signed 64-bit bytes. It observes cancellation. These are admission checks, not an LMDB integrity check or repair.
- Preserve the pier, logs and recovery/checkpoint evidence after a failure. This workaround does not justify clearing a recovery hold, forcing repeated starts, truncating a database, breaching a ship or discarding its event log.

## Evidence and regression pointers

The independent Vere 4.6 series had **one overlap failure in 16 default 500 GiB starts**, and **zero failures or overlaps in 16 explicit 64 GiB control starts**. Those numbers belong to the original Linux ARM64 synthetic fixture. They are not failure probabilities for production ships.

Harbor's subsequent native Mac ARM64 and Pi ARM64 trials each verified five starts: folder import at 64 GiB; manual starts selecting 128 GiB for a 40 GiB file and 256 GiB for a 100 GiB file; controller automatic startup at 256 GiB; and post-Pack restart at 256 GiB. Actual database mappings avoided the tested 2 GiB loom range, and authenticated login passed at each start. The larger files were valid copies with sparse trailing space, not populated large-ship workloads. Test managers stopped cleanly and their fixtures were removed; Pi fixtures used RAM storage.

[Runtime regressions](../internal/runtime/lmdb_test.go) cover thresholds, independent epochs, growth between starts, unsafe storage, cancellation and integer overflow. [Supervisor regressions](../internal/supervisor/supervisor_test.go) cover remeasurement and refusal before spawning, including guard release. All four Harbor targets built; native Intel execution, populated large ships, other loom configurations and renewed full comet replay acceptance remain open. See the [validation record](validation.md).

For the local selector/supervisor regressions, with the generated embedded UI already built:

```sh
go test ./internal/runtime -run TestLMDBMap -count=1
go test ./internal/supervisor \
  -run 'TestLaunchRecomputesMapFromDatabaseSize|TestUncheckedLMDBRefusesLaunchAndReleasesGuard' \
  -count=1
```

These tests verify Harbor's policy, not upstream address ownership. Retirement requires deterministic upstream mapping tests and native lifecycle validation.

## When it can be retired

There are two separate decisions: **retiring the collision mitigation** and **removing explicit database sizing**. A fixed Vere release can make the first unnecessary while an oversized default still makes the second useful on some hosts.

1. **Identify a released, verified upstream fix.** Record the release, fixing commit and per-platform artifact digests. Review that the proposal's ownership invariant holds before LMDB/substantial allocations in each relevant process, including exec-created workers, native and migration ranges, expansion/remapping, lite/full startup and utilities. A merged PR, version greater than 4.6, smaller default or the existence of `--lmdb-map-size` is insufficient.
2. **Prove address safety without Harbor's override.** Deterministic occupied-range/sentinel tests must expose the old behavior and show that the fixed code fails without overwriting existing memory. Exercise reservation before LMDB, conflicting expansion, migrations, demand/no-demand paths, utilities/replay/update and address-space exhaustion. Run native Linux/macOS ARM64 and x86_64 checks on the exact candidate artifacts. Repeat the retained fixture series without the override and inspect actual mappings and clean exits; successful ASLR trials alone are insufficient. Follow the proposal's full [regression coverage](upstream/vere-loom-reservation-proposal.md#regression-coverage).
3. **Separately prove default capacity and host compatibility.** Before dropping explicit sizing, validate that upstream defaults or its own sizing policy accommodate real database sizes and growth while fitting supported hosts, including the Pi. Test populated large piers and normal boot/import, manual/automatic starts, maintenance, replay and update/restart paths. Require clear failure under address-space exhaustion. If address safety is fixed but the default still cannot fit, retain a documented capacity policy rather than claiming continuing collision protection.
4. **Handle older and unverified runtimes deliberately.** The launch selector is unconditional; updating the catalog alone does not retire it. Review [the pinned catalog/resolver](../internal/runtime/download.go), [capability probing](../internal/runtime/capabilities.go), registered ship runtime paths and update/recovery behavior. Keep the workaround for older/unverified launchable artifacts, or stop supporting them explicitly before removing it globally. If support is mixed, bypass the mitigation only for artifacts with recorded fix and acceptance evidence; do not infer safety from an OS/CPU label or an arbitrary version comparison.
5. **Land removal with evidence and maintain the record.** Change the selector/supervisor policy and relevant regressions together. Check that fixed artifacts use the accepted upstream behavior and that any retained vulnerable artifacts still receive the override, including after runtime updates and automatic restarts. Run routine checks from [CONTRIBUTING.md](../CONTRIBUTING.md) and the native acceptance above. Update this status, architecture, onboarding, validation, release readiness and changelog with the retirement release/commit, artifact digests, tested platforms/paths and any retained sizing policy. Preserve the dated investigation and original evidence rather than rewriting historical results.

Until those gates are met, retain the workaround and its limits. This document records removal criteria; it does not claim that a later upstream release has been evaluated or fixed.
