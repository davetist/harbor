# External storage

Harbor supports piers and its own data on a mounted local drive. It verifies the configured mount path and filesystem UUID **before creating any data/pier directories, taking a Harbor lock, opening SQLite or starting jobs**. A missing mount, replacement filesystem, unavailable identity or path escaping the pinned filesystem refuses startup.

Configure the drive in the OS first, with a stable mount point and read/write access for the Harbor service user. On Linux, use a UUID-based mount and a filesystem suitable for Urbit, such as ext4. Harbor does not format or mount drives, edit fstab, grant permissions or configure swap. See the [Pi's external-storage guide](https://www.raspberrypi.com/documentation/computers/configuration.html#external-storage).

The service user needs write access to the chosen Harbor data folder and its parent, where Harbor keeps its controller lock. It also needs access to the permitted pier folders and their parent directories. Set this ownership during drive setup; Harbor does not grant itself permissions.

Keep Harbor's executable and configuration accessible independently of the external drive. Once the drive is mounted, obtain its filesystem UUID:

```sh
# Linux: identify the actual partition and mount point in this read-only list.
lsblk -o UUID,NAME,FSTYPE,SIZE,MOUNTPOINT,LABEL,MODEL

# macOS: read the Volume UUID of the mounted volume.
diskutil info /Volumes/Harbor
```

Replace the example UUID with the drive's actual **filesystem UUID**, not its partition UUID, label or device name:

```json
{
  "dataDir": "/mnt/harbor/harbor-data",
  "timezone": "Europe/Madrid",
  "requiredVolumes": [
    {
      "mountPoint": "/mnt/harbor",
      "uuid": "11111111-2222-3333-4444-555555555555"
    }
  ]
}
```

On macOS, use the canonical mounted path, for example `/Volumes/Harbor`, in both `mountPoint` and `dataDir`. Default permitted pier folders are inside `dataDir/piers`. This configuration puts Harbor's database, runtime cache, uploads, backups and default piers on the pinned volume. An existing pier is registered in place; keyfile boot creates a new pier under the selected destination.

You can keep Harbor's database locally and put only piers on the drive by setting an external `allowedRoots` path and pinning the volume that contains it. Each pin must contain `dataDir` or an allowed root. Paths under `/mnt`, `/media`, `/run/media` and `/Volumes` require a corresponding pin; pinning `/` cannot substitute for a missing drive under those paths. For a drive mounted at another location, explicitly add its `requiredVolumes` entry too: a path alone cannot express which absent drive you intended.

## Startup checks

- The expected mount point must exist as a canonical directory and be an actual mount in the service's namespace.
- The mounted filesystem UUID must match, case-insensitively. Reformatting or replacing a drive requires an intentional configuration update.
- Existing ancestors of new data/pier directories must remain inside that filesystem. Symlink escapes and an unexpected nested filesystem are rejected.
- All required volumes are checked before any startup directory creation. A failed check creates no replacement database or pier folders on the underlying filesystem.
- Harbor keeps read-only mount-directory descriptors open until it stops, and rechecks identity after opening them. Stop Harbor and its ships before unmounting the drive normally.

Linux reads `/proc/self/mountinfo` and matches the active filesystem's device numbers against `/dev/disk/by-uuid`; it needs no root access or new utility. If UUID metadata is unavailable, it refuses startup. macOS uses the built-in `diskutil info -plist` to obtain the mounted path and Volume UUID; failure to obtain metadata also refuses startup. The metadata checks have a 10-second startup context.

These are startup and path-identity checks. They do not promise recovery from a physical USB disconnect, forced unmount, failed enclosure, power loss or filesystem corruption. Startup never falls back to another filesystem when a required volume cannot be verified. Actual-drive performance and physical failure tests remain pending until the intended drive is available.

Archive uploads require a Harbor integrity manifest; arbitrary tar archives are unsupported. To migrate a complete pier from another runner, stop it, copy the folder including hidden files to the mounted drive, then register **Existing folder** with its original loom requirement. See [onboarding](onboarding.md).
