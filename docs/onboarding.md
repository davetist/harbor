# Onboarding and migration

Harbor's homepage shows registered ships, or onboarding when none exist. A keyfile boots fresh state; an existing pier preserves the ship's current state. They serve different purposes.

**Browse** navigates folders on the computer running Harbor. The Mac app uses the macOS folder browser; the web interface and Linux desktop use Harbor's directory dialog, with parent navigation, breadcrumbs, Home, typed paths and a hidden-folder toggle. Choosing a folder confirms the path; cancelling keeps the previous value. Browsing respects the Harbor service user's OS read permissions. Import and boot additionally require the destination to be within a configured `allowedRoots` storage location. When controlling a Pi from another computer, the directory dialog browses the Pi's folders.

## Boot with a key

Choose **Start with a key**, select the keyfile from your browser's computer, and choose a destination on the Harbor host under a permitted root. Harbor stages the key privately, resolves the verified native runtime, creates a new pier and waits for readiness. Consumed keys are removed after the job; abandoned keys are cleaned at startup after 24 hours. The new destination must not already exist.

A keyfile does not contain your apps, messages or previous pier history. To move a running identity, migrate its complete current pier instead. Avoid running two copies of the same identity.

## Try a comet

Choose **Try a comet**, choose a local folder label/destination and boot. Vere creates its identity. This is the intended disposable exploration path. Fresh native comet boot/login and stop/start have passed on Mac and Pi ARM64; Intel and valid disposable-key acceptance remain open. Initial mining/bootstrap can take several minutes or longer. On every supported target, Harbor starts new piers with a 64 GiB LMDB virtual map. Existing/imported piers select a map of at least twice the largest database's logical size, rounded up to a power of two with a 64 GiB minimum. It is recalculated on each start to allow growth; this virtual mapping does not allocate that amount of RAM or reserve the loom's addresses. The policy mitigates the measured upstream mapping failure; it does not repair an already corrupt database.

See the [LMDB mapping workaround](lmdb-mapping-workaround.md) for growth and address-space limits, supported maintenance utilities, unsupported replay/update-preparation paths and the criteria for retiring it after a verified upstream fix.

## Register an existing folder

Stop the ship in its previous runner. Copy or move the complete pier, including hidden `.urb` contents, to the Harbor host if necessary. Choose **Use an existing pier → Existing folder** and select its host path. Harbor registers it in place, checks writer ownership and preserves the confirmed loom requirement. It does not upload or copy that folder automatically, and does not lower the loom to fit the new machine.

When viewing Harbor remotely, a folder picker browses the Harbor host. A path on your laptop is not a path on a separate server. Local key/archive uploads are the file-transfer paths.

## Loom size

A 2 GiB loom is Urbit's state address-space capacity (`--loom 31`), not a 2 GiB disk allocation or a promise of 2 GiB resident RAM. Harbor currently supports loom exponents 30–32 (1, 2 and 4 GiB). Resident memory, utility working space, snapshots and event logs are separate. A drive adds storage, not RAM. An existing pier's loom must be confirmed accurately; shrinking it is not a migration shortcut.

## Export and restore

Use **Maintenance → Runtime & recovery → Export pier**. Harbor stops the source, produces a full integrity-checked `.tar.gz`, and leaves it intentionally stopped even after a Harbor restart. An export failure after Stop also leaves automatic restart disabled. Scheduled maintenance skips intentionally stopped ships.

Download the final archive and upload it through **Use an existing pier → Upload an archive** on the destination. Harbor validates the manifest, hashes, paths, links and space, restores into a fresh folder, chooses the destination's native runtime, and boots. Ordinary tar archives without a Harbor manifest are unsupported. Existing folders can still be registered directly.

Keep the source stopped. If it resumes and advances network history, create a fresh final export. Old archives are not safe live-network rollback points; successful integrity checks or fake-ship restores do not prove real-identity continuity. Consult [Urbit migration guidance](https://docs.urbit.org/user-manual/running/cloud-hosting) before moving an identity.

Exports contain pier files, not Harbor's own database/configuration or external object-storage media. Archives are unencrypted and private. Uploaded archives and exports are retained. Use the [retention preview and explicit cleanup command](retention.md) to account for storage and prune eligible files with job/recovery protections. There is no automatic archive eviction or cloud backup. Plan disk space for the compressed upload, extracted pier and later exports.

Runtime update/replay create their own local checkpoints and follow their own restart rules. Those are not migration exports.

## External drives

Both key boot and existing-pier registration work on a mounted permitted drive. Configure the mount/UUID pin first, then select the destination there. You can keep manager state locally and put piers externally, or put all state on the drive. See [external storage](external-storage.md) for exact configuration and remaining hardware validation.
