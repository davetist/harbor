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

## Remove a ship from Harbor

Choose **Manage → Overview → Remove from Harbor** after stopping the ship. The confirmation shows the complete pier path on the Harbor host. Removal unregisters the ship and clears its maintenance settings and Harbor history; its pier, runtime and exported archives stay on disk. Add the pier back later through **Use an existing pier → Existing folder**, confirming its original loom size again.

Active or queued jobs, tracked live processes and unresolved recovery holds prevent removal. Wait for work to finish or resolve the hold first. Removal does not free disk space or delete a ship's identity.

## When Open does not respond

**Open ship** uses the hostname or IP you opened Harbor with and the ship's current HTTP port. Harbor first checks the saved listener's public login page, with a two-second timeout. This checks the ship from the Harbor host; your browser still needs network access to that listener. A running process can be busy compiling apps or performing another long computation even when HTTP stops responding.

If the check fails, Harbor offers **Try Open again**, **Inspect terminal**, and **Interrupt current event…**. Waiting is appropriate while useful work is progressing. Interruption requires a confirmation: it sends Vere's supported Ctrl+C to cancel the current computation without restarting the ship. Canceled work may need to be retried. This is manual recovery, not a fix for the underlying Urbit computation, and Harbor never interrupts automatically after a timeout.

Before sending, Harbor checks the listener again and refuses if it responds. It also refuses during boot, stop, maintenance, queued work, recovery holds, shutdown, or uncertain process ownership. **Interrupt sent** confirms only that the input was sent. Wait briefly and choose **Try Open again** to verify that the ship responds. An existing recovery hold still needs its recorded outcome verified through the separate recovery flow.

## Loom size

A 2 GiB loom is Urbit's state address-space capacity (`--loom 31`), not a 2 GiB disk allocation or a promise of 2 GiB resident RAM. Harbor currently supports loom exponents 30–32 (1, 2 and 4 GiB). Resident memory, utility working space, snapshots and event logs are separate. A drive adds storage, not RAM. An existing pier's loom must be confirmed accurately; shrinking it is not a migration shortcut.

## Export and restore

Use **Maintenance → Runtime & recovery → Export pier**. Harbor stops the source, produces a full integrity-checked `.tar.gz`, and leaves it intentionally stopped even after a Harbor restart. An export failure after Stop also leaves automatic restart disabled. Scheduled maintenance skips intentionally stopped ships.

Download the final archive and upload it through **Use an existing pier → Upload an archive** on the destination. Harbor validates the manifest, hashes, paths, links and space, restores into a fresh folder, chooses the destination's native runtime, and boots. Ordinary tar archives without a Harbor manifest are unsupported. Existing folders can still be registered directly.

Keep the source stopped. If it resumes and advances network history, create a fresh final export. Old archives are not safe live-network rollback points; successful integrity checks or fake-ship restores do not prove real-identity continuity. Consult [Urbit migration guidance](https://docs.urbit.org/user-manual/running/cloud-hosting) before moving an identity.

Exports contain pier files, not Harbor's own database/configuration or external object-storage media. Archives are unencrypted and private. Uploaded archives and exports are retained. Use the [retention preview and explicit cleanup command](retention.md) to account for storage and prune eligible files with job/recovery protections. There is no automatic archive eviction or cloud backup. Plan disk space for the compressed upload, extracted pier and later exports.

Runtime update/replay create their own local checkpoints and follow their own restart rules. Those are not migration exports.

## External drives

Both key boot and existing-pier registration work on a mounted drive with the necessary OS permissions. Select the destination there directly. You can optionally configure a mount/UUID pin so Harbor refuses startup if that drive is missing or replaced. You can keep manager state locally and put piers externally, or put all state on the drive. See [external storage](external-storage.md) for pin configuration and remaining hardware validation.
