# Automatic maintenance

Harbor starts new registrations with maintenance enabled on the two measured hardware configurations below, using official Vere 4.6 and the original 2 GiB loom (`--loom 31`). The profile is a bounded native baseline, not a guarantee for every ship or evidence of an optimal long-term cadence.

| Operation | Default interval | Local time | Behavior |
| --- | --- | --- | --- |
| Pack | 7 calendar days | 03:00 | Compact state. |
| Meld | 28 calendar days | 03:05 | Deduplicate state only when the snapshot is at least 512 MiB and resource checks pass. |
| Roll | 7 calendar days | 03:10 | Create a new event-log epoch from the current snapshot; this can increase disk usage. |
| Chop | 7 calendar days | 03:15 | Run only after the successful Roll for that exact cycle. Keep Vere's newest two epochs and delete older event history. |

Times use the displayed host IANA timezone, anchored to the registration date. Operations are serialized; later times are eligibility times, not guaranteed start or completion times. Each operation has editable timing and a pause control. Pausing does not terminate an executing utility. Existing registrations retain their saved settings when Harbor is upgraded.

Automatic operations run in **Pack → conditional Meld → Roll → Chop** order. An enabled earlier stage scheduled for the same maintenance day must become eligible and settle before a later stage can run, even when its saved clock time is later. This also covers existing registrations with the former default times. An operation that is paused or scheduled for a later day is not added to the cycle.

After downtime, each schedule coalesces missed occurrences to its latest due occurrence. Harbor enqueues compaction before retention and checks the order again when dispatching the durable queue, including queues created by older controllers. A due Meld therefore precedes Roll even when their coalesced occurrences have different dates. A skipped resource check settles that stage with a visible skipped result; it does not establish successful compaction. Failed or uncertain utility outcomes retain their recovery holds and prevent further maintenance. The below-512-MiB Meld skip remains a normal conditional outcome.

Manual operations retain their ordinary queue behavior and resource/writer checks. Manual Chop does not require a preceding Pack, Meld or Roll, so it remains available for standalone history pruning when the ship has no active or queued job and no recovery hold.

Two epochs does **not** mean two weeks of history. Manual rolls and runtime migrations can change their spans. Chop reduces the history available for replay; a full backup preserves only the history present when it was taken. Harbor skips paired Chop when that cycle's Roll failed or is uncertain. It does not use a previous successful Roll as a substitute.

## Measured scope

- macOS ARM64: Apple M2 Max with at least 32 GiB physical RAM.
- Linux ARM64: Raspberry Pi 4 Model B Rev 1.4 with the 8 GB memory configuration (at least 7 GiB reported RAM).
- Both: official Vere 4.6, loom exponent 31; the native fixture began with a 766.6 MiB snapshot.

Other CPUs, Pi revisions, memory configurations, runtime versions and loom sizes have no measured default profile yet. New schedules on those configurations start paused and say why. macOS/Linux x86_64 passed fresh-comet Pack/Roll/Chop lifecycle acceptance in the [first alpha release pass](validation.md#first-named-alpha-release-pass--2026-10-13); measured automatic-maintenance profiles and native Meld acceptance there remain outstanding. Explicit manual Pack/Roll/Chop remain subject to resource checks; Meld additionally requires a measured hardware/runtime/loom profile.

## Checks before running

Harbor measures resources before stopping a ship and again before launching the utility. A failed first check skips a scheduled operation without stopping the ship; a manual request reports failure. If resources change after Stop, the second check refuses to launch the utility and leaves the ship stopped with a failed result. These are conservative admission rules, not reserved memory or completion guarantees.

| Operation | Minimum currently available RAM |
| --- | --- |
| Pack | 2 GiB |
| Meld | 4 GiB |
| Roll | 512 MiB |
| Chop | 256 MiB |

Every operation requires a readable, nonempty snapshot no larger than 1 GiB. The 1 GiB cap and RAM thresholds provide policy headroom above the measured fixture; larger states require further measurement. Scheduled Meld skips snapshots below 512 MiB. A user-requested manual Meld can run below that pressure threshold if its other checks pass.

Free space is checked on the **actual pier volume**, including an external drive: twice the current snapshot size plus 256 MiB for checkpoint/work space. Standalone Chop on an already stopped ship requires 64 MiB instead. This reserve does not promise enough space for arbitrary future growth or backups. Linux uses `MemAvailable`; macOS uses free, inactive and speculative pages as a conservative reclaimable-memory estimate. Neither treats swap as available RAM.

Harbor does not configure swap, mount or format storage, force-kill a utility, or change networking to make maintenance run. Utilities have no forced duration deadline. An intentionally stopped ship remains stopped when a scheduled occurrence is skipped. Successful maintenance restarts only a ship that was running beforehand.

Maintenance restarts use the same 20-minute readiness deadline as normal starts. If the runtime does not confirm readiness, Harbor finishes the maintenance job as failed and shows the ship as needing attention instead of leaving it indefinitely starting. The successful utility outcome, tracked runtime and recovery hold remain available for inspection; the readiness deadline does not force-kill a writer or release its pier guard. See the [scheduled restart verification](validation/maintenance-restarts.md).

## Why these defaults

On the disposable Pi fixture, the first Pack reclaimed 391.2 MiB of allocated space. Meld reclaimed another 10.1 MiB, then zero on two repeated passes, while still taking about 21 seconds each on the SD card. Roll added about 365 MiB per pass; paired Chop restored the two-epoch policy. This supports separating routine compaction, paired history retention, and less frequent conditional deduplication with less frequent deduplication.

The 7-day and 28-day intervals and 512 MiB threshold are initial conservative policy choices informed by those measurements. Three immediate repetitions cannot establish real daily/weekly growth, the best pressure metric, an optimal interval or behavior on representative active ships. Current [Vere guidance](https://docs.urbit.org/user-manual/running/vere) distinguishes Pack from the more memory-intensive Meld, and defines Roll/Chop's epoch effects; it does not establish these Harbor intervals.

Compaction precedes Roll because Vere copies the current snapshot into the new epoch. Melding afterward reduces the working snapshot but does not shrink that saved copy. This ordering improves the size of epoch snapshots; it does not imply that Vere requires a Meld before every Roll for correctness.

Pi measurements used its SD card. External SSD/HDD timings remain unmeasured; an external drive does not add RAM. Checks follow the actual pier volume, while performance and failure behavior need validation on the intended drive.

See [validation and limitations](validation.md).
