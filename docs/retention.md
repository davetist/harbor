# Retained uploads and exports

Harbor keeps archive uploads and full-pier exports until you explicitly prune them. Nothing in the default service configuration automatically removes those archives. Consumed boot keys are removed after their job; startup also removes abandoned keys older than 24 hours while preserving keys referenced by unfinished/failed work.

Use `harbor retention` to account for retained files and review a cleanup plan. Stop the Harbor service first. The command takes the same exclusive controller lock as the service, including canonical folder aliases, verifies required volumes, and requires an existing database; it will not create a replacement manager when storage is missing.

```sh
# Preview only. 0 days, the default, disables archive expiry.
harbor retention --config /absolute/config.json \
  --upload-days 7 --export-days 30 --keep-exports 2

# After reviewing the exact paths/reasons in the JSON report:
harbor retention --config /absolute/config.json \
  --upload-days 7 --export-days 30 --keep-exports 2 --apply
```

The report includes logical and allocated bytes, reclaimable bytes, remaining bytes and each file's action/reason. `--budget-bytes` adds a budget check: if protected/recent files still exceed it, apply refuses before removing anything. The budget uses logical retained-file bytes; filesystem overhead/free space remain separate. This is a cleanup budget, not a live upload/export quota or automatic eviction policy. Existing `maxArchiveBytes` still limits each archive upload.

Cleanup preserves:

- Uploads referenced by queued, executing, failed or interrupted jobs, including pending imports.
- All exports for a ship with active work or a recovery hold.
- At least the newest two existing exports that still pass their integrity checks for each ship. Corrupt/unverified exports are preserved for inspection and do not count toward that minimum. `--keep-exports` can increase it.
- Unknown files, unregistered exports, subdirectories and symlinks. Redirected managed upload/backup directories refuse cleanup.

Only Harbor-named regular files in its managed `uploads`/`backups` directories are eligible. Both an export's record and its file must be older than the requested age. Eligible files and the integrity-checked retained checkpoints are rechecked before deletion; changed targets refuse apply. Pruned registered exports are removed from the download list, while job history and recovery records remain intact. Piers, runtimes, SQLite and external folders are never cleanup targets.

Keep the report private: paths and archives can identify ships. Archives are unencrypted. Age is not proof that an archive is safe for a live-network rollback; retain the final stopped-source migration export until the destination is independently accepted. A manual cleanup may partially complete if storage fails during removal; inspect the reported error and rerun preview before trying again.
