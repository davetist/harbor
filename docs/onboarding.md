# Onboarding and migration

Harbor's homepage shows registered ships, or onboarding when none exist. A keyfile boots fresh state; an existing pier preserves the ship's current state. They serve different purposes.

**Browse** navigates folders on the computer running Harbor. The Mac app uses the macOS folder browser; the web interface and Linux desktop use Harbor's directory dialog, with parent navigation, breadcrumbs, Home, typed paths and a hidden-folder toggle. Choosing a folder confirms the path; cancelling keeps the previous value. You can create or register a pier in any folder the Harbor service user can access and write to, including mounted external drives. No Harbor folder whitelist is required. When controlling a Pi from another computer, the directory dialog browses the Pi's folders.

## Boot with a key

Choose **Start with a key**, select the keyfile from your browser's computer, and choose a destination on the Harbor host. Harbor stages the key privately, resolves the verified native runtime, creates a new pier and waits for readiness. Consumed keys are removed after the job; abandoned keys are cleaned at startup after 24 hours. The new destination must not already exist.

A keyfile does not contain your apps, messages or previous pier history. To move a running identity, migrate its complete current pier instead. Avoid running two copies of the same identity.

## Try a comet

Choose **Try a comet**, choose a local folder label/destination and boot. Vere creates its identity. This is the intended disposable exploration path. Fresh native comet boot/login, stop/start, Pack/Roll/Chop and export/restore/replay passed on all four macOS/Linux ARM64/x86_64 targets in the [first alpha release pass](validation.md#first-named-alpha-release-pass--2026-10-13). Valid disposable-key onboarding and real-identity migration remain open. Initial mining/bootstrap can take several minutes or longer. On every supported target, Harbor starts new piers with a 64 GiB LMDB virtual map. Existing/imported piers select a map of at least twice the largest database's logical size, rounded up to a power of two with a 64 GiB minimum. It is recalculated on each start to allow growth; this virtual mapping does not allocate that amount of RAM or reserve the loom's addresses. The policy mitigates the measured upstream mapping failure; it does not repair an already corrupt database.

See the [LMDB mapping workaround](lmdb-mapping-workaround.md) for growth and address-space limits, supported maintenance utilities, unsupported replay/update-preparation paths and the criteria for retiring it after a verified upstream fix.

If a start times out before readiness is confirmed, the job fails and the ship shows **Needs attention**. Harbor keeps tracking its runtime and holds the pier lock. Open the ship overview and choose **Start ship** to check that same process again. Inspect the terminal while bootstrap is still progressing. A maintenance recovery hold must be resolved through its separate recovery flow.

## Register an existing folder

Stop the ship in its previous runner. Copy or move the complete pier, including hidden `.urb` contents, to the Harbor host if necessary. Choose **Use an existing pier → Existing folder** and select its host path. Harbor registers it in place, checks writer ownership and preserves the confirmed loom requirement. It does not upload or copy that folder automatically, and does not lower the loom to fit the new machine.

When viewing Harbor remotely, a folder picker browses the Harbor host. A path on your laptop is not a path on a separate server. Local key/archive uploads are the file-transfer paths.

Pier and temporary-work folders need directory ancestry owned by the Harbor user or root and protected from other OS users' writes. Private folders beneath sticky shared temporary directories are supported; the pier and its native storage directories must themselves prevent other users' writes, even when sticky. Harbor refuses replaceable shared ancestry, linked native lock files and escaping or unavailable snapshot targets; choose a private folder instead of bypassing these checks. Safe relative snapshot links contained within the pier remain supported. Harbor retains its private `.harbor-guards-v1` folder beside managed directories for filesystem-equivalent ownership locks; do not delete guard files while a manager or ship is using them.

## Remove a ship from Harbor

Choose **Manage → Overview → Remove from Harbor** after stopping the ship. The confirmation shows the complete pier path on the Harbor host. By default, removal unregisters the ship and clears its maintenance settings and Harbor history; its pier, runtime and exported archives stay on disk. Add the preserved pier back later through **Use an existing pier → Existing folder**, confirming its original loom size again.

To also erase the pier, select **Permanently delete pier files**, review the path and irreversible-deletion warning, then confirm **Remove and delete pier**. This deletes the entire pier folder, including any mounted desk files inside it. Exported archives and shared runtimes remain on disk. Harbor refuses deletion while the ship has work, a recovery hold or another writer, or if the folder contains a mount point, another registered pier, Harbor storage, a runtime or a tracked export. Stop the ship first and move protected contents out before retrying.

Deletion records a durable hold before changing files. If storage fails or Harbor stops during deletion, the ship stays registered and cannot start; its automatic schedules stay paused. Resolve storage access, reopen removal and explicitly confirm permanent deletion again to finish. Harbor checks the original folder identity before retrying and preserves a replacement folder at the same path.

Active or queued jobs, tracked live processes and unresolved recovery holds prevent removal. Wait for work to finish or resolve the hold first. Removal does not free disk space or delete a ship's identity.

## When Open does not respond

**Open ship** uses the hostname or IP you opened Harbor with and the ship's current HTTP port. Harbor first checks the saved listener's public login page, with a two-second timeout. This checks the ship from the Harbor host; your browser still needs network access to that listener. A running process can be busy compiling apps or performing another long computation even when HTTP stops responding.

If the check fails, Harbor offers **Try Open again**, **Inspect terminal**, and **Interrupt current event…**. Waiting is appropriate while useful work is progressing. Interruption requires a confirmation: it sends Vere's supported Ctrl+C to cancel the current computation without restarting the ship. Canceled work may need to be retried. This is manual recovery, not a fix for the underlying Urbit computation, and Harbor never interrupts automatically after a timeout.

Before sending, Harbor checks the listener again and refuses if it responds. It also refuses during boot, stop, maintenance, queued work, recovery holds, shutdown, or uncertain process ownership. **Interrupt sent** confirms only that the input was sent. Wait briefly and choose **Try Open again** to verify that the ship responds. An existing recovery hold still needs its recorded outcome verified through the separate recovery flow.

## After an interrupted Start or Stop

If Harbor stopped before recording a Start or Stop operation's completion, the ship can remain under a recovery hold even after its process has exited. The detail page shows the hold's explanation ahead of any earlier startup error. Choose **Verify recorded outcome** on the ship detail page. The button changes to **Verifying…** while the check runs, which can take several minutes. Harbor checks the current pier without starting the ship or restoring an older archive, then refreshes the ship's displayed state.

An interrupted Start requires saved running intent and exactly one recorded ship launch whose process is confirmed gone, with its process and binary identities recorded. A retry of a failed readiness check can refer back through consecutive readiness timeouts to that same original launch; it does not need to have launched a second process. Another operation, a different failure, missing or uncertain launch evidence, another live writer, or an unexpected utility keeps the hold in place.

An interrupted Stop must have recorded a running ship and persisted stopped intent. Both recovery paths require no live or uncertain process, no pier writer, and successful offline inspection with the saved runtime. Harbor locks the original against writers and copies its current `.urb/log` and `.urb/chk` into a private temporary folder beside the pier. Vere inspects that copy with adaptive LMDB sizing. Any change to its data or directory structure, including falling back to an older epoch, rejects verification even if Vere exits successfully. LMDB reader-lock bookkeeping is excluded from comparison.

The check needs free space on the filesystem containing that temporary folder for the logical sizes of those event-log and snapshot files plus 64 MiB of headroom; sparse files may occupy their full logical size in the copy. Preparation or inspection failure keeps the hold and the original data. Harbor removes the temporary copy when the check finishes. An interrupted Harbor process may leave a private `.harbor-inspect-*` folder beside the pier; preserve it until its verification process is confirmed stopped. Success clears the hold and leaves the ship stopped; choose **Start ship** separately when ready. Do not clear database flags or delete locks to bypass recovery.

Offline inspection confirms that Vere can load an unchanged copy of the current snapshot and event store; it is not a full replay or a guarantee that every historical event is intact. Interrupted utilities and other operations retain their existing evidence requirements and may still require a separate recovery procedure. This inspection does not apply migrations or checkpoint repairs to the original pier.

## Loom size

A 2 GiB loom is Urbit's state address-space capacity (`--loom 31`), not a 2 GiB disk allocation or a promise of 2 GiB resident RAM. Harbor currently supports loom exponents 30–32 (1, 2 and 4 GiB). Resident memory, utility working space, snapshots and event logs are separate. A drive adds storage, not RAM. An existing pier's loom must be confirmed accurately; shrinking it is not a migration shortcut.

## Export and restore

Use **Maintenance → Runtime & recovery → Export pier**. Harbor stops the source, produces a full integrity-checked `.tar.gz`, and leaves it intentionally stopped even after a Harbor restart. An export failure after Stop also leaves automatic restart disabled. Scheduled maintenance skips intentionally stopped ships.

Download the final archive and upload it through **Use an existing pier → Upload an archive** on the destination. Harbor validates the manifest, hashes, paths, links and space, restores into a fresh folder, chooses the destination's native runtime, and boots. Ordinary tar archives without a Harbor manifest are unsupported. Existing folders can still be registered directly.

Archive validation and restore have a 30-minute parsing/work deadline, a 64 GiB payload limit, 100,000 files/directories including implicit parent directories, 4,096-byte paths/links, 255-byte components and 128 components per path. Each manifest and retained-path budget is limited to 32 MiB, and decompressed tar metadata (including hidden GNU/PAX records) to 128 MiB. Export enforces the same format limits. Restore reserves space for empty files/directories and checks available inodes when reported by the filesystem. Links must resolve within the archive without traversing a link as an extraction parent; case-aliased link parents are refused. Native lock/control files are verified but recreated after restore.

Keep the source stopped. If it resumes and advances network history, create a fresh final export. Old archives are not safe live-network rollback points; successful integrity checks or fake-ship restores do not prove real-identity continuity. Consult [Urbit migration guidance](https://docs.urbit.org/user-manual/running/cloud-hosting) before moving an identity.

Exports contain pier files, not Harbor's own database/configuration or external object-storage media. Archives are unencrypted and private. Uploaded archives and exports are retained. Use the [retention preview and explicit cleanup command](retention.md) to account for storage and prune eligible files with job/recovery protections. There is no automatic archive eviction or cloud backup. Plan disk space for the compressed upload, extracted pier and later exports.

Runtime update/replay create their own local checkpoints and follow their own restart rules. Those are not migration exports.

## External drives

Both key boot and existing-pier registration work on a mounted drive with the necessary OS permissions. Select the destination there directly. You can optionally configure a mount/UUID pin so Harbor refuses startup if that drive is missing or replaced. You can keep manager state locally and put piers externally, or put all state on the drive. See [external storage](external-storage.md) for pin configuration and remaining hardware validation.
